package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

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

	// 如果会话状态为 "open"，且未匹配到任何快捷回复，则检查宽限期
	if req.Conversation.Status == string(enum.ConversationStatusOpen) {
		gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ConversationID)
		err := global.RedisClient.Get(ctx, gracePeriodKey).Err()

		if err == nil {
			// 标志存在，说明在宽限期内，AI将继续处理新消息
			global.Log.Debugf("会话 %d 处于转人工宽限期，AI将继续处理新消息", req.Conversation.ConversationID)
			isGracePeriodOverride = true // 标记为宽限期处理模式
		} else if err == redis.ErrNil {
			// 标志不存在，正常退出，交由人工处理
			return
		} else {
			// 查询Redis时发生其他错误，为安全起见，直接退出
			global.Log.Errorf("检查会话 %d 的转人工宽限期标志失败: %v", req.Conversation.ConversationID, err)
			return
		}
	}

	// --- 快速路径未命中，进入慢速路径 --- //
	var (
		vectorResults       []dao.SearchResult
		conversationHistory []common.LlmMessage
		chosenVectorAnswer  string
	)

	g, gCtx := errgroup.WithContext(ctx)

	// 2. 慢速路径：并发执行向量搜索和历史记录获取
	g.Go(func() error {
		var currentVectorResults []dao.SearchResult
		var searchErr error
		currentVectorResults, searchErr = service.Service.UserServiceGroup.VectorService.Search(gCtx, req.Content)
		if searchErr != nil {
			global.Log.Warnf("[processMessageAsync] 向量数据库搜索失败: %v", searchErr)
			return nil
		}

		if len(currentVectorResults) > 0 {
			vectorResults = currentVectorResults
			// 检查是否有高相似度的向量搜索结果，并触发快速响应
			if currentVectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
				chosenVectorAnswer = currentVectorResults[0].Answer
				return ErrVectorMatchFound
			}
		}
		return nil
	})

	g.Go(func() error {
		var historyErr error
		conversationHistory, historyErr = d.getConversationHistory(gCtx, req.Conversation.AccountID, req.Conversation.ConversationID, req.Content)
		if historyErr != nil {
			global.Log.Errorf("[processMessageAsync] 获取会话历史记录失败: %v", historyErr)
			return historyErr
		}
		return nil
	})

	// 等待所有goroutine完成或被中断
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		switch {
		case errors.Is(err, ErrVectorMatchFound):
			// 向量搜索高相似度匹配，提前响应
			global.Log.Debugf("[processMessageAsync] 向量搜索高相似度匹配，提前响应, 相似度: %.4f, 会话ID: %d", vectorResults[0].Similarity, req.Conversation.ConversationID)
			service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, chosenVectorAnswer)
			go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, chosenVectorAnswer)
			return // 回复已发送，终止流程

		default:
			global.Log.Errorf("[processMessageAsync] errgroup执行失败: %v", err)
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
			return
		}
	}

	// --- 如果代码能执行到这里，说明没有任何快速路径被命中 --- //

	// 告诉用户“机器人正在输入中...”
	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)
	}()

	// 准备给LLM的参考资料
	var llmReferenceDocs []dao.SearchResult
	if len(vectorResults) > 0 {
		for _, res := range vectorResults {
			global.Log.Debugln(res.Similarity, "================相似度")
			// 只使用相似度高于配置阈值的文档作为参考
			if res.Similarity >= global.Config.Ai.VectorSearchMinSimilarity {
				llmReferenceDocs = append(llmReferenceDocs, res)
			}
		}
	}

	global.Log.Debugln("=================开始进入LLM")

	// 调用LLM服务获取回复
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.NewChat(ctx, &req, llmReferenceDocs, conversationHistory)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			global.Log.Debugf("会话 %d 的AI任务被取消。", req.Conversation.ConversationID)
			return
		}
		global.Log.Errorf("[processMessageAsync] LLM错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	if llmAnswer == "" {
		global.Log.Warnf("[processMessageAsync] LLM返回空回复，转人工, 会话ID: %d", req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5, string(enum.ReplyMsgLlmError))
		return
	}

	// 如果在宽限期内AI成功处理，则异步将会话状态改回“机器人”
	if isGracePeriodOverride {
		go func() {
			// 再次检查宽限期标志是否仍然存在
			gracePeriodKey := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, req.Conversation.ConversationID)
			err := global.RedisClient.Get(context.Background(), gracePeriodKey).Err() // 使用context.Background()，因为此协程独立于请求生命周期

			if err == redis.ErrNil {
				// 宽限期已过，或者标志已被删除，不再尝试改回bot状态，避免与人工客服冲突
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

	service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, llmAnswer)
	go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, llmAnswer)
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
