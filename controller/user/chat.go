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

func (c *ChatApi) HandleWebhook(ctx *gin.Context) {
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
	case chatwoot.EventWebwidgetTriggered:
		global.Log.Debugln("收到WebWidget触发事件:", string(bb))

		var req common.WebwidgetTriggeredRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			global.Log.Errorf("[HandleWebhook] 解析 WebWidgetPayload 失败: %v", err)
			return
		}
		if req.Contact.ID != 0 {
			go c.handleWebWidgetTriggered(req.Contact.ID, req.SourceID, req.Contact.CustomAttributes)
		}
		common.Success(ctx, nil)

	case chatwoot.EventMessageCreated:
		global.Log.Debugln(string(bb))
		var req common.ChatRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil || req.Conversation.ID == 0 {
			common.Fail(ctx, "参数无效")
			return
		}
		c.handleMessageCreated(ctx, req)

	case chatwoot.EventConversationResolved:
		var req common.ConversationResolvedRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil || req.ID == 0 {
			common.Fail(ctx, "参数无效")
			return
		}
		go c.handleConversationResolved(req.ID)
		common.Success(ctx, nil)

	default:
		common.Success(ctx, nil)
	}
}

// handleWebWidgetTriggered 复活旧会话或创建新会话，并发送卡片
func (c *ChatApi) handleWebWidgetTriggered(contactID uint, sourceID string, attrs common.CustomAttributes) {
	conversations, err := global.ChatwootService.GetContactConversations(contactID)
	if err != nil {
		global.Log.Errorf("获取联系人 %d 的会话列表失败: %v", contactID, err)
		return
	}

	var targetConversationID uint

	if len(conversations) == 0 {
		//新用户，无历史会话 -> 主动创建会话
		if sourceID == "" {
			global.Log.Warnf("联系人 %d 无历史会话且 Webhook 缺少 source_id，无法主动创建会话", contactID)
			return
		}
		global.Log.Debugf("联系人 %d 为新用户，正在主动创建会话...", contactID)
		newID, err := global.ChatwootService.CreateConversation(sourceID)
		if err != nil {
			global.Log.Errorf("为联系人 %d 创建新会话失败: %v", contactID, err)
			return
		}
		targetConversationID = newID
        global.Log.Debugf("成功为联系人 %d 创建新会话: %d", contactID, targetConversationID)

	} else {
		// 老用户 -> 取最近的一个会话
		lastConv := conversations[0]
		targetConversationID = lastConv.ID

		// 如果会话已解决，强制复活（改为 Open 状态）
		if lastConv.Status == chatwoot.ConversationStatusResolved {
			global.Log.Debugf("检测到用户 %d 重返，正在复活旧会话 %d", contactID, targetConversationID)
			if err := global.ChatwootService.SetConversationStatus(targetConversationID, chatwoot.ConversationStatusOpen); err != nil {
				global.Log.Errorf("复活会话 %d 失败: %v", targetConversationID, err)
				return
			}
		}
	}

	// 发送卡片 (利用之前加了锁的 ActionService)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service.Service.UserServiceGroup.ActionService.CheckAndSendProductCard(ctx, targetConversationID, attrs)
}

// handleMessageCreated 收到消息处理
func (c *ChatApi) handleMessageCreated(ctx *gin.Context, req common.ChatRequest) {
	// 处理"人工客服"消息: 将其计入Redis历史,并设置人工宽限期
	if req.MessageType == chatwoot.MessageTypeOutgoing && req.Sender.Type == chatwoot.SenderUser {
		service.Service.UserServiceGroup.ActionService.ActivateHumanModeGracePeriod(ctx.Request.Context(), req.Conversation.ID)

		common.Success(ctx, nil)
		if req.Content != "" {
			go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: req.Content})
		}
		return
	}

	// 处理非"用户"消息(即req.Conversation.Meta.Sender.Type=="agent_bot"机器人消息)
	if req.MessageType != chatwoot.MessageTypeIncoming || req.Conversation.Meta.Sender.Type != chatwoot.SenderContact {
		common.Success(ctx, nil)
		return
	}

	go func() {
		//理论上发送卡片的操作由webwidget_triggered事件处理，但为了避免不可预见的遗漏，这里再做一次
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		service.Service.UserServiceGroup.ActionService.CheckAndSendProductCard(bgCtx, req.Conversation.ID, req.Conversation.Meta.Sender.CustomAttributes)
	}()

	// 收到用户消息时，如果当前处于人工模式宽限期内，刷新宽限期时间
	service.Service.UserServiceGroup.ActionService.RefreshHumanModeGracePeriod(ctx.Request.Context(), req.Conversation.ID)

	// 调用验证器验证请求
	if err := service.Service.UserServiceGroup.Validator.ValidatorChatRequest(&req); err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	// 如果消息包含附件（图片、音视频等），则直接转人工
	if len(req.Attachments) > 0 {
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgUnsupportedAttachment))
		common.Fail(ctx, string(enum.ReplyMsgUnsupportedAttachment))
		return
	}

	// 提示词长度校验
	if utf8.RuneCountInString(req.Content) > int(global.Config.Ai.MaxPromptLength) {
		global.Log.Warnf("用户 %d 提问内容过长，已转人工", req.Conversation.ID)
		// 触发转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgPromptTooLong))
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
		c.storeTask(reqCopy.Conversation.ID, cancel)
		defer c.removeTask(reqCopy.Conversation.ID)

		c.processMessageAsync(asyncCtx, reqCopy)
	}()
}

// handleConversationResolved 处理会话解决事件，取消正在进行的AI任务
func (c *ChatApi) handleConversationResolved(conversationID uint) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()

	if cancel, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		cancel() // 调用取消函数
		delete(global.ActiveLLMTasks.Data, conversationID)
		global.Log.Debugf("会话%d已解决，已终止正在进行的AI任务。", conversationID)
	}
}

func (c *ChatApi) processMessageAsync(ctx context.Context, req common.ChatRequest) {
	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorf("[processMessageAsync] panic: %v", p)
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		}
	}()

	var isGracePeriodOverride bool // 标记是否处于宽限期处理模式

	// 1. 快速路径优先：同步执行关键词匹配
	cannedAnswer, isAction, err := service.Service.UserServiceGroup.ActionService.MatchCannedResponse(&req)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] 匹配关键字失败: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	// 转人工
	if isAction {
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman1, string(enum.ReplyMsgTransferSuccess))
		return
	}

	// 匹配到快捷回复
	if cannedAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ID, cannedAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: cannedAnswer})
		return
	}

	// 如果会话状态为 "open"，则检查是否需要由AI接管
	if req.Conversation.Status == chatwoot.ConversationStatusOpen {
		// 检查1：短时的“转人工宽限期”，用于AI在自动转人工后立即纠正
		transferGracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ID)
		err := global.RedisClient.Get(ctx, transferGracePeriodKey).Err()

		if err == nil { // 标志存在，AI可以覆盖转人工决定
			global.Log.Debugf("会话 %d 处于转人工宽限期，AI将继续处理新消息", req.Conversation.ID)
			isGracePeriodOverride = true
		} else if err != redis.ErrNil { // Redis查询出错
			global.Log.Errorf("检查会话 %d 的转人工宽限期标志失败: %v", req.Conversation.ID, err)
			return // 为安全起见，交由人工处理
		} else {
			// 标志不存在，继续检查长时的“人工模式宽限期”
			humanModeKey := fmt.Sprintf("%s%d", redis.KeyPrefixHumanModeActive, req.Conversation.ID)
			err := global.RedisClient.Get(ctx, humanModeKey).Err()

			if err == nil { // 标志存在，说明人工客服近期活跃
				global.Log.Debugf("会话 %d 处于人工模式宽限期，AI不介入。", req.Conversation.ID)
				return // 交由人工处理
			} else if err != redis.ErrNil { // Redis查询出错
				global.Log.Errorf("检查会话 %d 的人工模式宽限期标志失败: %v", req.Conversation.ID, err)
				return // 为安全起见，交由人工处理
			}

			// 如果两个宽限期标志都不存在，说明人工客服已长时间未参与，AI应该接管
			if err := service.Service.UserServiceGroup.ActionService.SetConversationPending(req.Conversation.ID); err != nil {
				global.Log.Errorf("尝试接管会话 %d 失败，无法将会话状态设置为 pending: %v", req.Conversation.ID, err)
				return // 接管失败，终止流程
			}
			global.Log.Debugf("会话 %d 状态为 'open' 但人工宽限期已过, 状态已成功切换至 pending，AI已接管。", req.Conversation.ID)
		}
	}

	// --- 进入智能处理路径 ---

	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ID, true)
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ID, false)
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
		fullHistory, historyErr = service.Service.UserServiceGroup.HistoryService.GetOrFetch(gCtx, req.Account.ID, req.Conversation.ID, req.Content)
		if historyErr != nil {
			global.Log.Warnf("[processMessageAsync] 获取历史记录失败: %v", historyErr)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		global.Log.Errorf("[processMessageAsync] 并发获取数据时发生意外错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	// 3. 高相似度直接回答
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		chosenVectorAnswer := vectorResults[0].Answer
		global.Log.Debugf("[processMessageAsync] 向量搜索高相似度匹配，提前响应, 相似度: %.4f, 会话ID: %d", vectorResults[0].Similarity, req.Conversation.ID)
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ID, chosenVectorAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: chosenVectorAnswer})
		return
	}

	// 如果向量搜索失败，清空可能存在的vectorResults，确保后续逻辑正确处理空结果
	if vectorErr != nil {
		vectorResults = nil
	}

	if global.Config.Ai.MaxLlmHistoryMessages > 0 && len(fullHistory) > int(global.Config.Ai.MaxLlmHistoryMessages) {
		startIndex := len(fullHistory) - int(global.Config.Ai.MaxLlmHistoryMessages)
		fullHistory = fullHistory[startIndex:]
		global.Log.Debugf("会话 %d 历史记录已限制为最近 %d 条消息", req.Conversation.ID, global.Config.Ai.MaxLlmHistoryMessages)
	}

	global.Log.Debugln("会话历史=========", fullHistory)

	// 4. 分诊台 (Triage) & 智能路由
	processed, err := c.runTriage(ctx, req, fullHistory, vectorResults)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] 分诊失败: %v, 会话ID: %d", err, req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}
	if processed {
		return
	}

	// --- 分诊通过，进入深度处理路径 ---

	// 5. 调用大型LLM服务 (含RAG和工具调用)
	llmAnswer, err := c.runComplexGeneration(ctx, req, fullHistory, vectorResults)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			global.Log.Debugf("会话 %d 的AI任务被取消。", req.Conversation.ID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 复杂路径处理失败: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	global.Log.Debugln("LLM回答=================", llmAnswer)

	// 6. 最终回复处理
	if strings.TrimSpace(llmAnswer) == enum.LlmUnsureTransferSignal {
		global.Log.Debugf("[processMessageAsync] LLM不确定答案，主动转人工, 会话ID: %d", req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman5, "")
		return
	}

	if llmAnswer == "" {
		global.Log.Warnf("[processMessageAsync] LLM返回空回复，转人工, 会话ID: %d", req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman5, string(enum.ReplyMsgLlmError))
		return
	}

	// 7. 如果在宽限期内AI成功处理，则异步将会话状态改回“机器人”
	if isGracePeriodOverride {
		go func() {
			gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ID)
			err := global.RedisClient.Get(context.Background(), gracePeriodKey).Err()
			if err == redis.ErrNil {
				global.Log.Debugf("会话 %d 宽限期已过，AI不再尝试改回bot状态。", req.Conversation.ID)
				return
			}
			if err != nil {
				global.Log.Warnf("重新检查会话 %d 宽限期标志失败: %v", req.Conversation.ID, err)
				return
			}
			// 宽限期标志仍然存在，可以安全地改回bot状态
			if err := service.Service.UserServiceGroup.ActionService.SetConversationPending(req.Conversation.ID); err != nil {
				global.Log.Warnf("将会话 %d 状态改回机器人失败: %v", req.Conversation.ID, err)
			} else {
				global.Log.Debugf("会话 %d 状态成功从open改回bot。", req.Conversation.ID)
			}
		}()
	}

	// 8. 发送消息并更新历史
	service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ID, llmAnswer)
	go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer})
}

// runTriage 执行分诊与智能路由
func (c *ChatApi) runTriage(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (processed bool, err error) {
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
		global.Log.Debugf("[Triage] 触发高优先级转人工规则, 意图: %s, 情绪: %s, 紧急度: %s, 会话ID: %d", triageResult.Intent, triageResult.Emotion, triageResult.Urgency, req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgTransferSuccess))
		return true, nil
	}

	if enum.TriageIntent(triageResult.Intent) == enum.TriageIntentOffTopic {
		global.Log.Debugf("[Triage] 识别为无关问题，已礼貌拒绝, 会话ID: %d", req.Conversation.ID)
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ID, string(enum.ReplyMsgOffTopic))
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: string(enum.ReplyMsgOffTopic)})
		return true, nil
	}

	return false, nil
}

// runComplexGeneration 执行复杂的RAG+LLM生成，并处理工具调用
func (c *ChatApi) runComplexGeneration(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (string, error) {
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
		global.Log.Debugf("[runComplexGeneration] LLM请求调用工具, 会话ID: %d", req.Conversation.ID)

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

// storeTask 存储一个异步任务的取消函数
func (c *ChatApi) storeTask(conversationID uint, cancel context.CancelFunc) {
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
func (c *ChatApi) removeTask(conversationID uint) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()
	delete(global.ActiveLLMTasks.Data, conversationID)
}
