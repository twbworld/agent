package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"

	"github.com/gin-gonic/gin"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/service"
)

type ChatApi struct{}

var ErrVectorMatchFound = errors.New("vector match found")

func (d *ChatApi) HandleChat(ctx *gin.Context) {
	bodyBytes, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		common.Fail(ctx, "参数无效")
		return
	}
	bb := bodyBytes

	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var eventFinder common.Event
	if err := json.Unmarshal(bodyBytes, &eventFinder); err != nil {
		common.Fail(ctx, "参数无效")
		return
	}

	switch chatwoot.ChatwootEvent(eventFinder.Event) {
	case chatwoot.EventMessageCreated:
		global.Log.Debugln(string(bb))
		var req common.ChatRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			common.Fail(ctx, "参数无效")
			return
		}
		d.handleMessageCreated(ctx, req)

	case chatwoot.EventConversationCreated:
		// global.Log.Debugln(string(bb))
		var req common.ConversationCreatedRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			common.Fail(ctx, "参数无效")
			return
		}
		go d.handleConversationCreated(ctx, req)
		common.Success(ctx, nil)

	case chatwoot.EventConversationResolved:
		var req common.ConversationResolvedRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			common.Fail(ctx, "参数无效")
			return
		}
		go d.handleConversationResolved(req.ID)
		common.Success(ctx, nil)

	default:
		common.Success(ctx, nil)
	}
}

// handleConversationCreated 处理新会话创建事件，这是用户发送第一条消息时触发的
func (d *ChatApi) handleConversationCreated(ctx *gin.Context, req common.ConversationCreatedRequest) {
	attrs := req.Meta.Sender.CustomAttributes
	if attrs.GoodsID == "" {
		return
	}
	service.Service.UserServiceGroup.ActionService.SendProductCard(req.ID, attrs)
}

// handleConversationResolved 收到消息处理
func (d *ChatApi) handleMessageCreated(ctx *gin.Context, req common.ChatRequest) {
	// 处理"人工客服"消息: 将其计入Redis历史
	if req.MessageType == chatwoot.MessageTypeOutgoing && req.Sender.Type == chatwoot.SenderUser {
		//转人工后不需要用到LLM,也就不需要历史记录;以防万一还是加上
		common.Success(ctx, nil)
		if req.Content != "" {
			go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ConversationID, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: req.Content})
		}
		return
	}

	// 处理非"用户"消息(即req.Conversation.Meta.Sender.Type=="agent_bot"机器人消息)
	if req.MessageType != chatwoot.MessageTypeIncoming || req.Conversation.Meta.Sender.Type != chatwoot.SenderContact {
		common.Success(ctx, nil)
		return
	}

	// 调用验证器验证请求
	if err := service.Service.UserServiceGroup.Validator.ValidatorChatRequest(&req); err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	// 如果消息包含附件（图片、音视频等），则直接转人工
	if len(req.Attachments) > 0 {
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman3, string(enum.ReplyMsgUnsupportedAttachment))
		common.Fail(ctx, string(enum.ReplyMsgUnsupportedAttachment))
		return
	}

	// 提示词长度校验
	if utf8.RuneCountInString(req.Content) > int(global.Config.Ai.MaxPromptLength) {
		global.Log.Warnf("用户 %d 提问内容过长，已转人工", req.Conversation.ConversationID)
		// 触发转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman3, string(enum.ReplyMsgPromptTooLong))
		common.Fail(ctx, string(enum.ReplyMsgPromptTooLong))
		return
	}

	// 回复消息已接收
	common.Success(ctx, nil)

	// 避免`req`在HTTP返回后可能被Gin回收。
	reqCopy := req

	go func() {
		timeout := time.Duration(global.Config.Ai.AsyncJobTimeout) * time.Second
		asyncCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// 注册任务，以便在会话解决时可以取消
		d.storeTask(reqCopy.Conversation.ConversationID, cancel)
		defer d.removeTask(reqCopy.Conversation.ConversationID)

		d.processMessageAsync(asyncCtx, reqCopy)
	}()
}

func (d *ChatApi) processMessageAsync(ctx context.Context, req common.ChatRequest) {
	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorf("[processMessageAsync] panic: %v", p)
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		}
	}()

	var isGracePeriodOverride bool // 标记是否处于宽限期处理模式

	// 1. 快速路径优先：同步执行关键词匹配
	cannedAnswer, isAction, err := service.Service.UserServiceGroup.ActionService.CannedResponses(&req)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] 匹配关键字失败: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	// 转人工
	if isAction {
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman1, string(enum.ReplyMsgTransferSuccess))
		return
	}

	// 匹配到快捷回复
	if cannedAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, cannedAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ConversationID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: cannedAnswer})
		return
	}

	// 如果会话状态为 "open"，则检查宽限期
	if req.Conversation.Status == chatwoot.ConversationStatusOpen {
		gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ConversationID)
		err := global.RedisClient.Get(ctx, gracePeriodKey).Err()

		if err == nil {
			global.Log.Debugf("会话 %d 处于转人工宽限期，AI将继续处理新消息", req.Conversation.ConversationID)
			isGracePeriodOverride = true
		} else if err == redis.ErrNil {
			return // 标志不存在，正常退出，交由人工处理
		} else {
			global.Log.Errorf("检查会话 %d 的转人工宽限期标志失败: %v", req.Conversation.ConversationID, err)
			return // 查询Redis时发生其他错误，为安全起见，直接退出
		}
	}

	// --- 进入智能处理路径 ---

	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)
	}()

	// 2. 并发获取向量搜索结果和会话历史
	var vectorResults []dao.SearchResult
	var fullHistory []common.LlmMessage
	var vectorErr error // 使用独立的错误变量，因为向量搜索失败不应中断整个流程

	g, gCtx := errgroup.WithContext(ctx)

	// 向量搜索
	g.Go(func() error {
		var searchErr error
		vectorResults, searchErr = service.Service.UserServiceGroup.VectorService.Search(gCtx, req.Content)
		if searchErr != nil && !errors.Is(searchErr, context.Canceled) {
			global.Log.Warnf("[processMessageAsync] 向量数据库搜索失败: %v", searchErr)
			vectorErr = searchErr
		}
		return nil
	})

	// 获取会话历史
	g.Go(func() error {
		var historyErr error
		fullHistory, historyErr = service.Service.UserServiceGroup.HistoryService.GetOrFetch(gCtx, req.Account.ID, req.Conversation.ConversationID, req.Content)
		if historyErr != nil {
			global.Log.Warnf("[processMessageAsync] 获取历史记录失败: %v", historyErr)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		global.Log.Errorf("[processMessageAsync] 并发获取数据时发生意外错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	// 3. 高相似度直接回答
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		chosenVectorAnswer := vectorResults[0].Answer
		global.Log.Debugf("[processMessageAsync] 向量搜索高相似度匹配，提前响应, 相似度: %.4f, 会话ID: %d", vectorResults[0].Similarity, req.Conversation.ConversationID)
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, chosenVectorAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ConversationID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: chosenVectorAnswer})
		return
	}

	// 如果向量搜索失败，清空可能存在的vectorResults，确保后续逻辑正确处理空结果
	if vectorErr != nil {
		vectorResults = nil
	}

	if global.Config.Ai.MaxLlmHistoryMessages > 0 && len(fullHistory) > int(global.Config.Ai.MaxLlmHistoryMessages) {
		startIndex := len(fullHistory) - int(global.Config.Ai.MaxLlmHistoryMessages)
		fullHistory = fullHistory[startIndex:]
		global.Log.Debugf("会话 %d 历史记录已限制为最近 %d 条消息", req.Conversation.ConversationID, global.Config.Ai.MaxLlmHistoryMessages)
	}

	global.Log.Debugln("会话历史=========", fullHistory)

	// 4. 分诊台 (Triage) & 智能路由
	processed, err := d.runTriage(ctx, req, fullHistory, vectorResults)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] 分诊失败: %v, 会话ID: %d", err, req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}
	if processed {
		return
	}

	// --- 分诊通过，进入深度处理路径 ---

	// 5. 调用大型LLM服务 (含RAG和工具调用)
	llmAnswer, err := d.runComplexGeneration(ctx, req, fullHistory, vectorResults)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			global.Log.Debugf("会话 %d 的AI任务被取消。", req.Conversation.ConversationID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 复杂路径处理失败: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	global.Log.Debugln("LLM回答=================", llmAnswer)

	// 6. 最终回复处理
	if strings.TrimSpace(llmAnswer) == enum.LlmUnsureTransferSignal {
		global.Log.Debugf("[processMessageAsync] LLM不确定答案，主动转人工, 会话ID: %d", req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5, "")
		return
	}

	if llmAnswer == "" {
		global.Log.Warnf("[processMessageAsync] LLM返回空回复，转人工, 会话ID: %d", req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5, string(enum.ReplyMsgLlmError))
		return
	}

	// 7. 如果在宽限期内AI成功处理，则异步将会话状态改回“机器人”
	if isGracePeriodOverride {
		go func() {
			gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ConversationID)
			err := global.RedisClient.Get(context.Background(), gracePeriodKey).Err()
			if err == redis.ErrNil {
				global.Log.Debugf("会话 %d 宽限期已过，AI不再尝试改回bot状态。", req.Conversation.ConversationID)
				return
			}
			if err != nil {
				global.Log.Warnf("重新检查会话 %d 宽限期标志失败: %v", req.Conversation.ConversationID, err)
				return
			}
			// 宽限期标志仍然存在，可以安全地改回bot状态
			if err := service.Service.UserServiceGroup.ActionService.SetConversationPending(req.Conversation.ConversationID); err != nil {
				global.Log.Warnf("将会话 %d 状态改回机器人失败: %v", req.Conversation.ConversationID, err)
			} else {
				global.Log.Debugf("会话 %d 状态成功从open改回bot。", req.Conversation.ConversationID)
			}
		}()
	}

	// 8. 发送消息并更新历史
	service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, llmAnswer)
	go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ConversationID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer})
}

// runTriage 执行分诊与智能路由
func (d *ChatApi) runTriage(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (processed bool, err error) {
	// 准备分诊台所需的上下文信息
	var triageHistory []common.LlmMessage
	if len(fullHistory) > 0 {
		// 取最后4条消息 (相当于2轮完整对话) 作为分诊上下文
		const triageHistoryLimit = 4
		startIndex := len(fullHistory) - triageHistoryLimit
		if startIndex < 0 {
			startIndex = 0
		}
		triageHistory = fullHistory[startIndex:]
	}

	var retrievedQuestions []string
	if len(vectorResults) > 0 {
		// 只取前N个最相关的问题作为上下文，避免prompt过长
		for i, res := range vectorResults {
			if i >= int(global.Config.Ai.TriageContextQuestions) {
				break
			}
			retrievedQuestions = append(retrievedQuestions, res.Question)
		}
	}

	triageCtx, triageCancel := context.WithTimeout(ctx, 10*time.Second) // 为分诊步骤设置一个较短的超时
	defer triageCancel()

	triageResult, err := service.Service.UserServiceGroup.LlmService.Triage(triageCtx, req.Content, triageHistory, retrievedQuestions)
	if err != nil {
		return false, err
	}

	global.Log.Debugf("=================分诊结果: %+v", triageResult)

	// 根据分诊结果执行路由
	triggerTransferEmotions := []enum.TriageEmotion{
		enum.TriageEmotionAngry,
		enum.TriageEmotionFrustrated,
		enum.TriageEmotionAnxious,
	}
	triggerTransferUrgencies := []enum.TriageUrgency{
		enum.TriageUrgencyCritical,
		enum.TriageUrgencyHigh,
	}

	if utils.InSlice(triggerTransferEmotions, enum.TriageEmotion(triageResult.Emotion)) > -1 ||
		enum.TriageIntent(triageResult.Intent) == enum.TriageIntentRequestHuman ||
		utils.InSlice(triggerTransferUrgencies, enum.TriageUrgency(triageResult.Urgency)) > -1 {
		global.Log.Debugf("[Triage] 触发高优先级转人工规则, 意图: %s, 情绪: %s, 紧急度: %s, 会话ID: %d", triageResult.Intent, triageResult.Emotion, triageResult.Urgency, req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman3, string(enum.ReplyMsgTransferSuccess))
		return true, nil
	}

	if enum.TriageIntent(triageResult.Intent) == enum.TriageIntentOffTopic {
		global.Log.Debugf("[Triage] 识别为无关问题，已礼貌拒绝, 会话ID: %d", req.Conversation.ConversationID)
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, string(enum.ReplyMsgOffTopic))
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ConversationID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: string(enum.ReplyMsgOffTopic)})
		return true, nil
	}

	return false, nil
}

// runComplexGeneration 执行复杂的RAG+LLM生成，并处理工具调用
func (d *ChatApi) runComplexGeneration(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (string, error) {
	// 准备给大型LLM的参考资料 (RAG)
	var llmReferenceDocs []dao.SearchResult
	if len(vectorResults) > 0 {
		for _, res := range vectorResults {
			// 只使用相似度高于配置阈值的文档作为参考
			if res.Similarity >= global.Config.Ai.VectorSearchMinSimilarity {
				llmReferenceDocs = append(llmReferenceDocs, res)
			}
		}
	}

	global.Log.Debugln("=================开始进入大型LLM")

	conversationHistory := fullHistory
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.GenerateResponseOrToolCall(ctx, &req, llmReferenceDocs, conversationHistory)
	if err != nil {
		return "", err // 将错误传递给上层处理
	}

	// 检查是否需要调用工具
	if strings.Contains(llmAnswer, "<tool_code>") {
		global.Log.Debugln("=================需调用Mcp")
		global.Log.Debugf("[runComplexGeneration] LLM请求调用工具, 会话ID: %d", req.Conversation.ConversationID)

		if global.McpService != nil {
			// 将 toolCodeBlock 的声明和使用都放在这个if块内，避免McpService为nil时出现“声明但未使用”的警告
			toolCodeBlock := strings.TrimSpace(strings.Split(strings.Split(llmAnswer, "<tool_code>")[1], "</tool_code>")[0])

			var toolCalls common.ToolCalls
			if err := json.Unmarshal([]byte(toolCodeBlock), &toolCalls); err != nil {
				global.Log.Errorf("[runComplexGeneration] 解析工具调用JSON数组失败: %v", err)
				toolResult := fmt.Sprintf("工具调用格式错误: %v", err)
				conversationHistory = append(conversationHistory, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer}, common.LlmMessage{Role: openai.ChatMessageRoleTool, Content: toolResult})
			} else if len(toolCalls) > 0 {
				// 从MCP服务获取所有工具的描述
				toolDescriptions := global.McpService.GetToolDescriptions()

				// 并发执行所有工具调用
				var toolResults []common.LlmMessage
				var mu sync.Mutex
				g, gCtx := errgroup.WithContext(ctx)
				g.SetLimit(5) // 限制并发数为5，防止过多请求冲击MCP服务

				for _, toolCall := range toolCalls {
					toolCall := toolCall // 避免闭包陷阱
					g.Go(func() error {
						var toolResultContent string
						parts := strings.SplitN(toolCall.Name, ".", 2)
						if len(parts) != 2 {
							toolResultContent = fmt.Sprintf("工具名称格式错误，必须为 '客户端名称.工具名称'，实际为: '%s'", toolCall.Name)
							global.Log.Errorf("[runComplexGeneration] %s", toolResultContent)
						} else {
							clientName, toolName := parts[0], parts[1]
							result, err := global.McpService.ExecuteTool(gCtx, clientName, toolName, toolCall.Arguments)
							if err != nil {
								toolResultContent = fmt.Sprintf("工具 '%s' 调用失败: %v", toolCall.Name, err)
								global.Log.Errorf("[runComplexGeneration] %s", toolResultContent)
							} else {
								toolResultContent = result
								global.Log.Debugf("=================成功获取Mcp数据 for '%s': %s", toolCall.Name, toolResultContent)
							}
						}

						// 获取工具描述
						toolDescription := "未知工具"
						if desc, ok := toolDescriptions[toolCall.Name]; ok {
							toolDescription = desc
						}

						// 为每个工具结果创建一个结构化的消息，并安全地追加到结果切片中
						finalContent := fmt.Sprintf(
							"[工具名称]: %s\n[工具作用]: %s\n[返回结果]:\n%s",
							toolCall.Name,
							toolDescription,
							toolResultContent,
						)
						mu.Lock()
						toolResults = append(toolResults, common.LlmMessage{
							Role:    openai.ChatMessageRoleTool,
							Content: finalContent,
						})
						mu.Unlock()
						return nil
					})
				}

				// 等待所有工具调用完成
				if err := g.Wait(); err != nil {
					// errgroup 本身返回的错误通常是第一个非nil的错误，这里只记录日志
					global.Log.Errorf("[runComplexGeneration] 执行MCP工具组时发生错误: %v", err)
				}

				// 将用户问题、助手回复（工具调用指令）和所有工具执行结果一起添加到历史记录中
				conversationHistory = append(conversationHistory, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content})
				conversationHistory = append(conversationHistory, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer})
				conversationHistory = append(conversationHistory, toolResults...)
			}

			global.Log.Debugln("=================再次调用大型LLM分析数据")

			// 将工具执行结果和历史记录再次发送给LLM进行总结
			llmAnswer, err = service.Service.UserServiceGroup.LlmService.SynthesizeToolResult(ctx, conversationHistory)
			if err != nil {
				return "", fmt.Errorf("工具调用后LLM错误: %w", err)
			}
		}
	}

	return llmAnswer, nil
}

// handleConversationResolved 处理会话解决事件，取消正在进行的AI任务
func (d *ChatApi) handleConversationResolved(conversationID uint) {
	// 对于 conversation_resolved 事件, conversation ID 在根对象的 ID 字段
	if conversationID == 0 {
		global.Log.Warnf("从 conversation_resolved 事件中未能获取到有效的 conversation_id")
		return
	}

	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()

	if cancel, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		cancel() // 调用取消函数
		delete(global.ActiveLLMTasks.Data, conversationID)
		global.Log.Debugf("会话%d已解决，已终止正在进行的AI任务。", conversationID)
	}
}

// storeTask 存储一个异步任务的取消函数
func (d *ChatApi) storeTask(conversationID uint, cancel context.CancelFunc) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()
	// 如果该会话已有任务在运行，先取消旧的
	if oldCancel, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		oldCancel()
		global.Log.Debugf("会话 %d 的旧AI任务已被新任务取代并取消。", conversationID)
	}
	global.ActiveLLMTasks.Data[conversationID] = cancel
}

// removeTask 移除一个已完成或已取消的异步任务
func (d *ChatApi) removeTask(conversationID uint) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()
	delete(global.ActiveLLMTasks.Data, conversationID)
}
