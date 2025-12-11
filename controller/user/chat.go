package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	// bb := bodyBytes

	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var eventFinder common.Event
	if err := json.Unmarshal(bodyBytes, &eventFinder); err != nil {
		common.Fail(ctx, "参数无效")
		return
	}

	switch chatwoot.ChatwootEvent(eventFinder.Event) {
	case chatwoot.EventWebwidgetTriggered:
		// global.Log.Debugln("收到WebWidget触发事件:", string(bb))

		var req common.WebwidgetTriggeredRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			global.Log.Errorf("[HandleWebhook] 解析 WebWidgetPayload 失败: %v", err)
			return
		}
		if req.Contact.ID != 0 {
			go c.handleWebWidgetTriggered(context.Background(), req.Contact.ID, req.SourceID, req.Contact.CustomAttributes)
		}
		common.Success(ctx, nil)

	case chatwoot.EventMessageCreated:
		// global.Log.Debugln(string(bb))
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
func (c *ChatApi) handleWebWidgetTriggered(ctx context.Context, contactID uint, sourceID string, attrs common.CustomAttributes) {
	conversations, err := global.ChatwootService.GetContactConversations(ctx, contactID)
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
		newID, err := global.ChatwootService.CreateConversation(ctx, sourceID)
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
			if err := global.ChatwootService.SetConversationStatus(ctx, targetConversationID, chatwoot.ConversationStatusOpen); err != nil {
				global.Log.Errorf("复活会话 %d 失败: %v", targetConversationID, err)
				return
			}
		}
	}

	if targetConversationID > 0 {
		// 并发执行上下文相关操作：发送卡片和分配团队
		go func() {
			actionCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			// 发送卡片
			service.Service.UserServiceGroup.ActionService.CheckAndSendProductCard(actionCtx, targetConversationID, attrs)
			// 根据规则分配团队 (currentTeamID为nil，因为此事件中无法得知)
			service.Service.UserServiceGroup.ActionService.AssignConversationByRules(actionCtx, targetConversationID, attrs, nil)
		}()
	}
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

	// 将上下文属性和当前团队ID提取出来，用于后续操作
	customAttrs := req.Conversation.Meta.Sender.CustomAttributes
	var currentTeamID *int
	if req.Conversation.Meta.Team != nil {
		currentTeamID = &req.Conversation.Meta.Team.ID
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// 理论上发送卡片的操作由webwidget_triggered事件处理，但为了避免不可预见的遗漏，这里再做一次
		service.Service.UserServiceGroup.ActionService.CheckAndSendProductCard(bgCtx, req.Conversation.ID, customAttrs)
		// 每次收到消息时，都检查是否需要根据最新上下文重新分配团队
		service.Service.UserServiceGroup.ActionService.AssignConversationByRules(bgCtx, req.Conversation.ID, customAttrs, currentTeamID)
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
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx.Request.Context(), req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgUnsupportedAttachment))
		common.Fail(ctx, string(enum.ReplyMsgUnsupportedAttachment))
		return
	}

	// 提示词长度校验
	if utf8.RuneCountInString(req.Content) > int(global.Config.Ai.MaxPromptLength) {
		global.Log.Warnf("用户 %d 提问内容过长，已转人工", req.Conversation.ID)
		// 触发转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx.Request.Context(), req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgPromptTooLong))
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
		// 注意: 不要在这里 defer cancel()，因为我们需要将 cancel 传递给全局 map 管理
		// 但是，为了防止内存泄漏（如果 storeTask 失败或任务被立即覆盖），我们必须确保最终会调用 cancel
		// 这里的逻辑改为：在 cleanupTask 中调用 cancel，或者在被替换时调用 oldCancel

		// 注册任务，以便在会话解决时可以取消
		// 存储任务并获取"我是唯一拥有者"的凭证 (MessageID)
		c.storeTask(reqCopy.Conversation.ID, reqCopy.ID, cancel)

		// 统一处理任务结束时的清理工作
		defer func() {
			// 在任务结束时（无论是正常结束、panic还是超时），尝试清理
			// 只有当当前任务仍然是 map 中的 active 任务时（通过 ID 匹配），才执行清理操作（如关闭 typing）
			// 如果 map 中已经存储了新的任务（ID不匹配），则说明当前任务是被中断或过期的，不应触碰全局状态
			isOwner := c.cleanupTask(reqCopy.Conversation.ID, reqCopy.ID)

			if isOwner {
				// 只有拥有者才有资格关闭 Typing 状态
				go service.Service.UserServiceGroup.ActionService.ToggleTyping(context.Background(), reqCopy.Conversation.ID, false)
				global.Log.Debugf("会话 %d 的AI任务(MsgID: %d)正常结束，清理完成。", reqCopy.Conversation.ID, reqCopy.ID)
			} else {
				global.Log.Debugf("会话 %d 的AI任务(MsgID: %d)已被新任务取代或过期，静默退出，不关闭Typing。", reqCopy.Conversation.ID, reqCopy.ID)
			}

			// 确保上下文被取消，释放资源
			cancel()
		}()

		c.processMessageAsync(asyncCtx, reqCopy)
	}()
}

// handleConversationResolved 处理会话解决事件，取消正在进行的AI任务
func (c *ChatApi) handleConversationResolved(conversationID uint) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()

	if taskInfo, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		taskInfo.Cancel() // 调用取消函数
		delete(global.ActiveLLMTasks.Data, conversationID)
		global.Log.Debugf("会话%d已解决，已终止正在进行的AI任务(MsgID: %d)。", conversationID, taskInfo.MessageID)
	}
}

func (c *ChatApi) processMessageAsync(ctx context.Context, req common.ChatRequest) {
	defer func() {
		if p := recover(); p != nil {
			// 优先检查Context是否已取消，确保被取代的任务能够静默退出
			if ctx.Err() == context.Canceled {
				global.Log.Debugf("会话 %d 任务(MsgID: %d)因被取代而取消，忽略 Panic 复原: %v", req.Conversation.ID, req.ID, p)
				return
			}
			global.Log.Errorf("[processMessageAsync] panic: %v", p)
			// 仅在Context未被取消时，才因Panic转人工，防止竞态条件
			if ctx.Err() != context.Canceled {
				_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
			}
		}
	}()

	var isGracePeriodOverride bool // 标记是否处于宽限期处理模式

	// 1. 快速路径优先：同步执行关键词匹配
	cannedAnswer, isAction, err := service.Service.UserServiceGroup.ActionService.MatchCannedResponse(&req)
	if err != nil {
		// 优先检查Context是否已取消
		if errors.Is(err, context.Canceled) {
			global.Log.Debugf("会话 %d 任务在关键词匹配前被取消，静默退出。", req.Conversation.ID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 匹配关键字失败: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	// 转人工
	if isAction {
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman1, string(enum.ReplyMsgTransferSuccess))
		return
	}

	// 匹配到快捷回复
	if cannedAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(ctx, req.Conversation.ID, cannedAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: cannedAnswer})
		return
	}

	// --- 统筹判断：转人工宽限期与人工模式 ---
	// 无论当前会话状态如何，优先检查是否存在"转人工宽限期"标志。
	transferGracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ID)
	err = global.RedisClient.Get(ctx, transferGracePeriodKey).Err()

	if err == nil {
		// 标志存在，AI可以覆盖转人工决定 (Override)，无论 Chatwoot 认为当前是 Pending 还是 Open
		global.Log.Debugf("会话 %d 处于转人工宽限期，AI将继续处理新消息", req.Conversation.ID)
		isGracePeriodOverride = true
	} else {
		// 宽限期标志不存在，执行常规状态检查
		if errors.Is(err, context.Canceled) {
			return
		}
		if req.Conversation.Status == chatwoot.ConversationStatusOpen {
			humanModeKey := fmt.Sprintf("%s%d", redis.KeyPrefixHumanModeActive, req.Conversation.ID)
			err := global.RedisClient.Get(ctx, humanModeKey).Err()
			if errors.Is(err, context.Canceled) {
				return
			}

			if err == nil { // 标志存在，说明人工客服近期活跃
				global.Log.Debugf("会话 %d 处于人工模式宽限期，AI不介入。", req.Conversation.ID)
				return // 交由人工处理
			} else if err != redis.ErrNil { // Redis查询出错
				global.Log.Errorf("检查会话 %d 的人工模式宽限期标志失败: %v", req.Conversation.ID, err)
				return // 为安全起见，交由人工处理
			}

			// 如果两个宽限期标志都不存在，说明人工客服已长时间未参与，AI应该接管
			if err := service.Service.UserServiceGroup.ActionService.SetConversationPending(ctx, req.Conversation.ID); err != nil {
				global.Log.Errorf("尝试接管会话 %d 失败，无法将会话状态设置为 pending: %v", req.Conversation.ID, err)
				return // 接管失败，终止流程
			}
			global.Log.Debugf("会话 %d 状态为 'open' 但人工宽限期已过, 状态已成功切换至 pending，AI已接管。", req.Conversation.ID)
		}
	}

	// --- 进入智能处理路径 ---
	go service.Service.UserServiceGroup.ActionService.ToggleTyping(ctx, req.Conversation.ID, true)

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
		if historyErr != nil && !errors.Is(historyErr, context.Canceled) {
			global.Log.Warnf("[processMessageAsync] 获取历史记录失败: %v", historyErr)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			global.Log.Debugf("会话 %d 的数据获取任务被取消（新消息介入），静默停止。", req.Conversation.ID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 并发获取数据时发生意外错误: %v", err)
		if ctx.Err() != context.Canceled {
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		}
		return
	}

	// 3. 高相似度直接回答
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		chosenVectorAnswer := vectorResults[0].Answer
		global.Log.Debugf("[processMessageAsync] 向量搜索高相似度匹配，提前响应, 相似度: %.4f, 会话ID: %d", vectorResults[0].Similarity, req.Conversation.ID)
		service.Service.UserServiceGroup.ActionService.SendMessage(ctx, req.Conversation.ID, chosenVectorAnswer)
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: chosenVectorAnswer})
		return
	}

	// 如果向量搜索失败，清空可能存在的vectorResults，确保后续逻辑正确处理空结果
	if vectorErr != nil {
		vectorResults = nil
	}

	// 按轮数修剪，同时限制单轮最大Assistant消息数，防止Token溢出或上下文被Bot刷屏
	if global.Config.Ai.ChatMaxHistoryRounds > 0 {
		// 从配置中获取单轮最大助手消息数，并提供默认值保护
		maxAssistant := int(global.Config.Ai.MaxAssistantPerRound)
		if maxAssistant <= 0 {
			maxAssistant = 5
		}
		fullHistory = c.trimHistory(fullHistory, int(global.Config.Ai.ChatMaxHistoryRounds), maxAssistant)
	}

	global.Log.Debugln("会话历史=========", fullHistory)
	global.Log.Debugln("RAG数量=========", len(vectorResults))

	// 4. 分诊台 (Triage) & 智能路由
	processed, err := c.runTriage(ctx, req, fullHistory, vectorResults)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			global.Log.Debugf("会话 %d 的分诊任务被取消，静默退出。", req.Conversation.ID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 分诊失败: %v, 会话ID: %d", err, req.Conversation.ID)
		if ctx.Err() != context.Canceled {
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		}
		return
	}
	if processed {
		return
	}

	// --- 分诊通过，进入深度处理路径 ---

	// 5. 调用大型LLM服务 (含RAG和工具调用)
	// 修改：接收返回的中间工具消息，以便存入历史
	llmAnswer, intermediateMsgs, err := c.runComplexGeneration(ctx, req, fullHistory, vectorResults)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			global.Log.Debugf("会话 %d 的AI任务被新任务取代而取消，静默退出。", req.Conversation.ID)
			return
		}
		global.Log.Errorf("[processMessageAsync] 复杂路径处理失败: %v", err)
		if ctx.Err() != context.Canceled {
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		}
		return
	}

	global.Log.Debugln("LLM回答=================", llmAnswer)

	// 6. 最终回复处理
	if strings.TrimSpace(llmAnswer) == enum.LlmUnsureTransferSignal {
		global.Log.Debugf("[processMessageAsync] LLM不确定答案，主动转人工, 会话ID: %d", req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman5, "")
		return
	}

	if llmAnswer == "" {
		global.Log.Warnf("[processMessageAsync] LLM返回空回复，转人工, 会话ID: %d", req.Conversation.ID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman5, string(enum.ReplyMsgLlmError))
		return
	}

	// 7. 如果在宽限期内AI成功处理，则异步将会话状态安全地改回“机器人”
	if isGracePeriodOverride {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// 再次检查宽限期标志，以防万一
			gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ID)
			err := global.RedisClient.Get(bgCtx, gracePeriodKey).Err()
			if err != nil { // 无论是 redis.ErrNil 还是其他错误，都意味着我们不应该再操作
				if err != redis.ErrNil {
					global.Log.Warnf("重新检查会话 %d 宽限期标志失败: %v", req.Conversation.ID, err)
				}
				return
			}
			// 宽限期标志仍然存在，可以安全地改回bot状态
			if err := service.Service.UserServiceGroup.ActionService.SetConversationPending(bgCtx, req.Conversation.ID); err != nil {
				global.Log.Warnf("在宽限期内将会话 %d 状态改回机器人失败: %v", req.Conversation.ID, err)
			} else {
				global.Log.Debugf("在宽限期内AI成功响应，已将会话 %d 状态从open改回pending。", req.Conversation.ID)
			}
		}()
	}

	// 8. 发送消息并更新历史
	// 将用户消息、中间工具调用过程(如有)和最终回复一并按顺序追加到Redis历史中
	service.Service.UserServiceGroup.ActionService.SendMessage(ctx, req.Conversation.ID, llmAnswer)
	go func() {
		historyToAppend := make([]common.LlmMessage, 0, 2+len(intermediateMsgs))
		// 先追加用户消息
		historyToAppend = append(historyToAppend, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content})
		// 追加中间的工具交互消息(Assistant Tool Call + Tool Results)
		if len(intermediateMsgs) > 0 {
			historyToAppend = append(historyToAppend, intermediateMsgs...)
		}
		// 最后追加Assistant的最终回复
		historyToAppend = append(historyToAppend, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer})
		service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, historyToAppend...)
	}()
}

// storeTask 存储一个异步任务的取消函数，并取消旧任务
func (c *ChatApi) storeTask(conversationID, messageID uint, cancel context.CancelFunc) {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()
	// 如果该会话已有任务在运行，先取消旧的
	if oldTask, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		oldTask.Cancel()
		global.Log.Debugf("会话 %d 的旧AI任务(MsgID: %d)已被新任务(MsgID: %d)取代并取消。", conversationID, oldTask.MessageID, messageID)
	}
	global.ActiveLLMTasks.Data[conversationID] = global.TaskInfo{
		Cancel:    cancel,
		MessageID: messageID,
	}
}

// cleanupTask 尝试清理任务。只有当 map 中存储的任务仍然是传入的 messageID 时，才执行删除并返回 true。
func (c *ChatApi) cleanupTask(conversationID, messageID uint) bool {
	global.ActiveLLMTasks.Lock()
	defer global.ActiveLLMTasks.Unlock()

	if task, exists := global.ActiveLLMTasks.Data[conversationID]; exists {
		if task.MessageID == messageID {
			delete(global.ActiveLLMTasks.Data, conversationID)
			return true
		}
	}
	return false
}

// runTriage 执行分诊与智能路由
func (c *ChatApi) runTriage(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (processed bool, err error) {
	// 准备分诊台所需的上下文信息，分诊台仅需极简上下文，单轮限制1条回复以节省Token
	var triageHistory []common.LlmMessage
	if len(fullHistory) > 0 {
		triageHistoryLimit := int(global.Config.Ai.TriageMaxHistoryRounds)
		triageHistory = c.trimHistory(fullHistory, triageHistoryLimit, 1)
	}

	var retrievedQuestions []string
	// 保存用于匹配的原始向量结果引用，以便后续直接提取答案
	var triageContextResults []dao.SearchResult

	if len(vectorResults) > 0 {
		limit := int(global.Config.Ai.TriageContextQuestions)
		if len(vectorResults) < limit {
			limit = len(vectorResults)
		}
		triageContextResults = vectorResults[:limit]
		// 仅遍历截取后的切片来提取问题文本
		for _, res := range triageContextResults {
			retrievedQuestions = append(retrievedQuestions, res.Question)
		}
	}

	triageTimeout := time.Duration(global.Config.Ai.TriageTimeout) * time.Second
	triageCtx, triageCancel := context.WithTimeout(ctx, triageTimeout)
	defer triageCancel()

	triageResult, err := service.Service.UserServiceGroup.LlmService.Triage(triageCtx, req.Content, triageHistory, retrievedQuestions, req.Conversation.Meta.Sender)
	if err != nil {
		return false, err
	}

	global.Log.Debugf("=================分诊结果: %+v", triageResult)

	// 优先处理高风险/转人工路由
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
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(ctx, req.Conversation.ID, enum.TransferToHuman3, string(enum.ReplyMsgTransferSuccess))
		return true, nil
	}

	// 2. 处理无关问题
	if enum.TriageIntent(triageResult.Intent) == enum.TriageIntentOffTopic {
		global.Log.Debugf("[Triage] 识别为无关问题，已礼貌拒绝, 会话ID: %d", req.Conversation.ID)
		service.Service.UserServiceGroup.ActionService.SendMessage(ctx, req.Conversation.ID, string(enum.ReplyMsgOffTopic))
		go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: string(enum.ReplyMsgOffTopic)})
		return true, nil
	}

	// 检查语义匹配结果 (AnswerID)
	// 如果小模型认为第 N 个问题与用户问题语义等同，则直接使用该问题的答案。
	if triageResult.AnswerID > 0 {
		index := triageResult.AnswerID - 1
		if index >= 0 && index < len(triageContextResults) {
			chosenAnswer := triageContextResults[index].Answer
			global.Log.Debugf("[Triage] 小模型语义匹配命中 (ID: %d), 意图: %s, 直接回复, 跳过大型LLM", triageResult.AnswerID, triageResult.Intent)

			service.Service.UserServiceGroup.ActionService.SendMessage(ctx, req.Conversation.ID, chosenAnswer)
			go service.Service.UserServiceGroup.HistoryService.Append(context.Background(), req.Conversation.ID, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content}, common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: chosenAnswer})

			return true, nil
		} else {
			global.Log.Warnf("[Triage] 模型返回了越界的 AnswerID: %d, 可用数量: %d", triageResult.AnswerID, len(triageContextResults))
		}
	}

	return false, nil
}

// runComplexGeneration 执行复杂的RAG+LLM生成，并处理工具调用
// 返回: 最终回复内容, 中间产生的消息历史(用于存入Redis), 错误
func (c *ChatApi) runComplexGeneration(ctx context.Context, req common.ChatRequest, fullHistory []common.LlmMessage, vectorResults []dao.SearchResult) (string, []common.LlmMessage, error) {
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
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.GenerateResponseOrToolCall(ctx, &req, llmReferenceDocs, conversationHistory, req.Conversation.Meta.Sender)

	// 用于收集本轮对话中产生的中间消息(工具调用请求+工具结果)，以便后续追加到Redis历史
	var intermediateMsgs []common.LlmMessage

	// 检查是否需要调用工具
	if strings.Contains(llmAnswer, "<tool_code>") {
		global.Log.Debugf("[runComplexGeneration] LLM请求调用工具, 会话ID: %d", req.Conversation.ID)

		// 记录工具调用指令(Assistant角色)
		assistantMsg := common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: llmAnswer}
		intermediateMsgs = append(intermediateMsgs, assistantMsg)

		toolResults, execErr := service.Service.UserServiceGroup.LlmService.ExecuteToolCalls(ctx, llmAnswer)
		if execErr != nil {
			global.Log.Errorf("[runComplexGeneration] 工具执行过程出错: %v", execErr)
			// 出错不打断流程，让LLM根据错误信息（已包含在toolResults中）尝试恢复或告知用户
		}

		if len(toolResults) > 0 {
			// 记录工具执行结果(Tool角色)
			intermediateMsgs = append(intermediateMsgs, toolResults...)

			// 将用户问题、助手回复（工具调用指令）和所有工具执行结果一起添加到历史记录中，用于最终合成
			conversationHistory = append(conversationHistory, common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: req.Content})
			conversationHistory = append(conversationHistory, assistantMsg)
			conversationHistory = append(conversationHistory, toolResults...)

			global.Log.Debugln("=================再次调用大型LLM分析数据")

			// 将工具执行结果和历史记录再次发送给LLM进行总结
			llmAnswer, err = service.Service.UserServiceGroup.LlmService.SynthesizeToolResult(ctx, conversationHistory)
		}
	}

	return llmAnswer, intermediateMsgs, err
}

// trimHistory 根据轮数和每轮最大消息数修剪历史记录
// maxRounds: 包含的最大用户消息数（即轮数）
// maxAssistantPerRound: 每个用户消息后保留的最大Assistant消息数（防止单轮消息过多）
func (c *ChatApi) trimHistory(history []common.LlmMessage, maxRounds int, maxAssistantPerRound int) []common.LlmMessage {
	if len(history) == 0 {
		return history
	}
	if maxRounds <= 0 {
		return []common.LlmMessage{}
	}

	var result []common.LlmMessage
	rounds := 0
	assistantCount := 0

	// 倒序遍历，确保保留最近的消息
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]

		if msg.Role == openai.ChatMessageRoleUser {
			rounds++
			// 如果超过了允许的最大轮数，则停止
			if rounds > maxRounds {
				break
			}
			// 重置助手消息计数器，因为我们已经进入了一个新的（按时间顺序是更早的）轮次
			// 注意：此时assistantCount统计的是当前这个User消息 *之后* 的Assistant消息
			assistantCount = 0

			result = append(result, msg)
		} else {
			// 对于非用户消息（助手、系统、工具等），我们作为该轮的一部分进行计数
			// 限制每轮Assistant消息的数量，避免Token被冗余回复占满
			if assistantCount < maxAssistantPerRound {
				result = append(result, msg)
				assistantCount++
			}
		}
	}

	// 倒序结果以恢复按时间顺序排列
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}
