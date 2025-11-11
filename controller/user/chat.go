package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/utils"
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
	// fmt.Println(string(bodyBytes))
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var eventFinder common.Event
	if err := json.Unmarshal(bodyBytes, &eventFinder); err != nil {
		common.Fail(ctx, "参数无效")
		return
	}

	switch eventFinder.Event {
	case enum.EventMessageCreated:
		var req common.ChatRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			common.Fail(ctx, "参数无效")
			return
		}
		d.handleMessageCreated(ctx, req)

	case enum.EventConversationResolved:
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

func (d *ChatApi) handleMessageCreated(ctx *gin.Context, req common.ChatRequest) {
	// 兼容; 新版 webhook 结构中, account_id 不在 conversation 内, 在根对象上
	if req.Account.ID != 0 {
		req.Conversation.AccountID = req.Account.ID
	}

	// 处理"人工客服"消息: 将其计入Redis历史
	if req.MessageType == string(enum.MessageTypeOutgoing) && req.Sender.Type == "user" {
		//理论上不会进入此处, 因为转人工后不需要用到LLM,也就不需要历史记录;以防万一
		common.Success(ctx, nil)
		reqCopy := req
		go d.updateHistoryWithHumanMessage(reqCopy)
		return
	}

	// 处理非"用户"消息
	if req.MessageType != string(enum.MessageTypeIncoming) || req.Conversation.Meta.Sender.Type != string(enum.SenderTypeContact) {
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
		go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, cannedAnswer)
		return
	}

	// 如果会话状态为 "open"，则检查宽限期
	if req.Conversation.Status == string(enum.ConversationStatusOpen) {
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

	// --- 进入智能处理路径 --- //

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
		fullHistory, historyErr = d.getConversationHistory(gCtx, req.Conversation.AccountID, req.Conversation.ConversationID, req.Content)
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
		go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, chosenVectorAnswer)
		return
	}

	// 如果向量搜索失败，清空可能存在的vectorResults，确保后续逻辑正确处理空结果
	if vectorErr != nil {
		vectorResults = nil
	}

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

	// --- 分诊通过，进入深度处理路径 --- //

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
	go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, llmAnswer)
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
		go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, string(enum.ReplyMsgOffTopic))
		return true, nil
	}

	return false, nil
}

// runComplexGeneration 执行复杂的RAG+LLM生成，并处理工具调用
func (d *ChatApi) runComplexGeneration(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (string, error) {
	// 告诉用户“机器人正在输入中...”
	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)
	}()

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

		// 从回复中提取工具调用代码（JSON）
		toolCode := strings.TrimSpace(strings.Split(strings.Split(llmAnswer, "<tool_code>")[1], "</tool_code>")[0])

		if global.McpService != nil {
			var toolCallParams common.ToolCallParams
			if err := json.Unmarshal([]byte(toolCode), &toolCallParams); err != nil {
				// 如果工具调用请求的JSON格式错误，记录错误并将格式错误信息作为工具结果
				global.Log.Errorf("[runComplexGeneration] 解析工具调用JSON失败: %v", err)
				toolResult := fmt.Sprintf("工具调用格式错误: %v", err)
				// 构建包含用户问题、助手回复和工具结果的完整历史
				conversationHistory = append(conversationHistory, common.LlmMessage{Role: "user", Content: req.Content}, common.LlmMessage{Role: "assistant", Content: llmAnswer}, common.LlmMessage{Role: "tool", Content: toolResult})
			} else {
				parts := strings.SplitN(toolCallParams.Name, ".", 2)
				if len(parts) != 2 {
					// 如果工具名称格式不正确，记录错误并返回相应提示
					toolResult := fmt.Sprintf("工具名称格式错误，必须为 '客户端名称.工具名称'，实际为: '%s'", toolCallParams.Name)
					global.Log.Errorf("[runComplexGeneration] %s", toolResult)
					conversationHistory = append(conversationHistory, common.LlmMessage{Role: "user", Content: req.Content}, common.LlmMessage{Role: "assistant", Content: llmAnswer}, common.LlmMessage{Role: "tool", Content: toolResult})
				} else {
					// 执行工具调用
					clientName, toolName := parts[0], parts[1]
					toolResult, err := global.McpService.ExecuteTool(ctx, clientName, toolName, toolCallParams.Arguments)
					if err != nil {
						// 如果工具执行失败，记录错误并返回失败信息
						global.Log.Errorf("[runComplexGeneration] 执行MCP工具 '%s' 失败: %v", toolCallParams.Name, err)
						toolResult = fmt.Sprintf("工具调用失败: %v", err)
					}
					global.Log.Debugln("=================成功获取Mcp数据: %s", toolResult)
					// 将用户问题、助手回复（工具调用）和工具执行结果一起添加到历史记录中
					conversationHistory = append(conversationHistory, common.LlmMessage{Role: "user", Content: req.Content}, common.LlmMessage{Role: "assistant", Content: llmAnswer}, common.LlmMessage{Role: "tool", Content: toolResult})
				}
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

// fetchAndCacheHistory 是一个辅助函数，用于从Chatwoot获取数据、格式化并存入Redis
func (d *ChatApi) fetchAndCacheHistory(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error) {
	// 从Chatwoot API获取完整的历史记录
	chatwootMessages, err := global.ChatwootService.GetConversationMessages(accountID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("从Chatwoot API获取会话 %d 消息失败: %w", conversationID, err)
	}

	// 格式化历史记录为LLM需要的格式
	var formattedHistory []common.LlmMessage
	for _, msg := range chatwootMessages {
		// 过滤掉私信备注、没有内容的附件消息
		if msg.Private || msg.Content == "" {
			continue
		}

		isIncoming := msg.MessageType == 0
		isOutgoing := msg.MessageType == 1

		// 过滤掉当前用户消息，因为它会作为LLM的content参数传入，避免重复
		if isIncoming && msg.Sender.Type == string(enum.SenderTypeContact) && msg.Content == currentMessage {
			continue
		}

		var role string
		if isIncoming && msg.Sender.Type == string(enum.SenderTypeContact) {
			role = "user"
		} else if isOutgoing {
			role = "assistant" // 假设所有outgoing消息都是AI或客服的回复
		} else {
			continue // 忽略其他类型的消息
		}
		formattedHistory = append(formattedHistory, common.LlmMessage{Role: role, Content: msg.Content})
	}

	// 将格式化后的历史记录存入Redis，并设置带抖动的过期时间
	ttl := utils.GetTTLWithJitter(global.Config.Redis.ConversationHistoryTTL)
	if err := global.RedisClient.SetConversationHistory(context.Background(), conversationID, formattedHistory, ttl); err != nil {
		// 只记录错误，不阻塞主流程返回
		global.Log.Errorf("将会话 %d 历史记录存入Redis失败: %v", conversationID, err)
	}

	return formattedHistory, nil
}

// getConversationHistory 获取会话历史记录，优先从Redis获取，否则从Chatwoot API获取
func (d *ChatApi) getConversationHistory(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error) {
	if global.RedisClient == nil {
		return nil, fmt.Errorf("Redis客户端未初始化")
	}

	// 1. 尝试从Redis获取聊天记录
	history, err := global.RedisClient.GetConversationHistory(ctx, conversationID)
	if err != nil && err != redis.ErrNil { // Redis error other than miss
		global.Log.Warnf("从Redis获取会话 %d 历史记录失败: %v, 将尝试从Chatwoot获取", conversationID, err)
	} else if history != nil { // Cache hit
		global.Log.Debugf("会话 %d 历史记录从Redis缓存命中", conversationID)
		return history, nil
	}

	// --- 缓存未命中，进入回源逻辑 ---
	if global.ChatwootService == nil {
		return nil, fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 2. 使用分布式锁防止缓存击穿
	lockKey := fmt.Sprintf("%s%d", redis.KeyPrefixHistoryLock, conversationID)
	lockExpiry := time.Duration(global.Config.Redis.HistoryLockExpiry) * time.Second
	agentID, _ := os.Hostname()
	if agentID == "" {
		agentID = "unknown-agent"
	}

	locked, err := global.RedisClient.SetNX(ctx, lockKey, agentID, lockExpiry).Result()
	if err != nil {
		global.Log.Errorf("尝试获取会话 %d 历史记录锁失败: %v", conversationID, err)
		// 即使获取锁失败，也尝试从源获取，作为降级策略
		return d.fetchAndCacheHistory(ctx, accountID, conversationID, currentMessage)
	}

	if locked {
		// 2a. 成功获取锁，从Chatwoot API获取数据并缓存
		global.Log.Debugf("会话 %d 历史记录Redis缓存未命中，成功获取锁，从Chatwoot API获取", conversationID)
		defer func() {
			// 使用后台 context 确保即使原始请求取消，锁释放也能执行
			if err := global.RedisClient.Del(context.Background(), lockKey).Err(); err != nil {
				global.Log.Warnf("释放会话 %d 历史记录锁失败: %v", conversationID, err)
			}
		}()
		// 在获取锁后，再次检查缓存，防止在获取锁的过程中，已有其他请求完成了缓存填充（双重检查锁定）
		history, err := global.RedisClient.GetConversationHistory(ctx, conversationID)
		if err == nil && history != nil {
			global.Log.Debugf("获取锁后发现会话 %d 缓存已存在", conversationID)
			return history, nil
		}
		return d.fetchAndCacheHistory(ctx, accountID, conversationID, currentMessage)
	}

	// 2b. 未获取到锁，说明其他goroutine正在回源，等待后重试
	global.Log.Debugf("会话 %d 历史记录锁被占用，等待后重试", conversationID)
	time.Sleep(200 * time.Millisecond) // 短暂等待

	history, err = global.RedisClient.GetConversationHistory(ctx, conversationID)
	if err == nil && history != nil {
		global.Log.Debugf("等待后，会话 %d 历史记录从Redis缓存命中", conversationID)
		return history, nil
	}

	// 如果等待后仍然没有缓存，作为降级策略，直接从源获取数据
	global.Log.Warnf("等待后会话 %d 缓存仍未命中，直接回源作为降级策略", conversationID)
	return d.fetchAndCacheHistory(ctx, accountID, conversationID, currentMessage)
}

// updateConversationHistory 在LLM回复后，更新Redis中的会话历史记录
func (d *ChatApi) updateConversationHistory(conversationID uint, userMessage, aiResponse string) {
	if global.RedisClient == nil {
		global.Log.Warnf("Redis客户端未初始化，无法更新会话 %d 历史记录", conversationID)
		return
	}

	ttl := utils.GetTTLWithJitter(global.Config.Redis.ConversationHistoryTTL)

	// 统一追加用户消息和AI回复
	messagesToAppend := []common.LlmMessage{
		{Role: "user", Content: userMessage},
		{Role: "assistant", Content: aiResponse},
	}

	if err := global.RedisClient.AppendToConversationHistory(context.Background(), conversationID, ttl, messagesToAppend...); err != nil {
		global.Log.Errorf("追加用户消息和AI回复到会话 %d 历史记录失败: %v", conversationID, err)
	}
}

// updateHistoryWithHumanMessage 将人工客服的回复追加到Redis历史记录中
func (d *ChatApi) updateHistoryWithHumanMessage(req common.ChatRequest) {
	if global.RedisClient == nil {
		global.Log.Warnf("Redis客户端未初始化，无法更新会话 %d 的人工客服历史记录", req.Conversation.ConversationID)
		return
	}
	if req.Content == "" {
		return
	}

	ttl := utils.GetTTLWithJitter(global.Config.Redis.ConversationHistoryTTL)
	messageToAppend := common.LlmMessage{
		Role:    "assistant",
		Content: req.Content,
	}

	if err := global.RedisClient.AppendToConversationHistory(context.Background(), req.Conversation.ConversationID, ttl, messageToAppend); err != nil {
		global.Log.Errorf("追加人工客服消息到会话 %d 历史记录失败: %v", req.Conversation.ConversationID, err)
	}
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
