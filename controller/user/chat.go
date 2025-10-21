package user

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/service"
)

type ChatApi struct{}

func (d *ChatApi) HandleChat(ctx *gin.Context) {
	var req common.ChatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Fail(ctx, "参数无效")
		return
	}

	// 兼容; 新版 webhook 结构中, account_id 不在 conversation 内, 在根对象上
	if req.Account.ID != 0 {
		req.Conversation.AccountID = req.Account.ID
	}

	// 仅处理 message_created 事件, 忽略其他事件如 conversation_status_changed 等
	if req.Event != string(enum.EventMessageCreated) {
		common.Success(ctx, nil)
		return
	}

	// 处理"人工客服"消息: 将其计入Redis历史
	if req.MessageType == string(enum.MessageTypeOutgoing) && req.Sender.Type == "user" && req.Sender.ID != global.Config.Chatwoot.AgentUserID {
		common.Success(ctx, nil) // 快速响应 webhook
		reqCopy := req
		go d.updateHistoryWithHumanMessage(reqCopy)
		return
	}

	// "用户"消息
	if req.MessageType != string(enum.MessageTypeIncoming) || req.Conversation.Meta.Sender.Type != string(enum.SenderTypeContact) {
		common.Success(ctx, nil)
		return
	}

	// 调用验证器验证请求
	if err := service.Service.UserServiceGroup.Validator.ValidatorChatRequest(&req); err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	// 如果会话状态为 "pending"，说明正在等待人工, AI不进行任何处理
	if req.Conversation.Status == string(enum.ConversationStatusPending) {
		common.Success(ctx, nil)
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
		d.processMessageAsync(asyncCtx, reqCopy)
	}()
}

func (d *ChatApi) processMessageAsync(ctx context.Context, req common.ChatRequest) {
	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorf("[processMessageAsync] panic: %v", p)
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgSystemError))
		}
	}()

	var (
		cannedAnswer        string
		isAction            bool
		vectorResults       []dao.SearchResult
		conversationHistory []common.LlmMessage
	)

	g, gCtx := errgroup.WithContext(ctx)
	// 1. 关键词匹配
	g.Go(func() error {
		var err error
		cannedAnswer, isAction, err = service.Service.UserServiceGroup.ActionService.CannedResponses(&req)
		if err != nil {
			global.Log.Errorf("[processMessageAsync] 匹配关键字失败: %v", err)
			return err
		}
		return nil
	})
	// 2. 向量搜索
	g.Go(func() error {
		var err error
		vectorResults, err = service.Service.UserServiceGroup.VectorService.Search(gCtx, req.Content)
		if err != nil {
			// 向量搜索失败是可接受的，LLM可以作为兜底
			global.Log.Warnf("[processMessageAsync] 向量数据库搜索失败: %v", err)
		}
		return nil
	})
	// 3. 获取会话历史记录
	g.Go(func() error {
		var err error
		conversationHistory, err = d.getConversationHistory(gCtx, req.Conversation.AccountID, req.Conversation.ConversationID, req.Content)
		if err != nil {
			global.Log.Errorf("[processMessageAsync] 获取会话历史记录失败: %v", err)
			return err
		}
		return nil
	})

	// 等待快速路径搜索和历史记录获取完成
	if err := g.Wait(); err != nil {
		// 如果是关键词匹配的错误，则转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgSystemError))
		return
	}

	// 1. 检查是否触发了动作（如转人工）
	if isAction {
		// 如果是转人工，则不需要更新Redis中的历史记录，因为后续会由人工处理
		return
	}
	// 2. 检查是否有精确匹配的答案
	if cannedAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, cannedAnswer)
		go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, cannedAnswer)
		return
	}

	global.Log.Debugln(vectorResults[0].Similarity, "==============", vectorResults[0].Question)

	// 3. 检查是否有高相似度的向量搜索结果
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, vectorResults[0].Answer)
		go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, vectorResults[0].Answer)
		return
	}

	// 告诉用户“机器人正在输入中...”
	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)
	}()

	// 准备给LLM的参考资料
	var llmReferenceDocs []dao.SearchResult
	if len(vectorResults) > 0 {
		for _, res := range vectorResults {
			// 只使用相似度高于配置阈值的文档作为参考
			if res.Similarity >= global.Config.Ai.VectorSearchMinSimilarity {
				llmReferenceDocs = append(llmReferenceDocs, res)
			}
		}
	}

	// 调用LLM服务获取回复
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.NewChat(ctx, &req, llmReferenceDocs, conversationHistory)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] LLM错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgLlmError))
		return
	}

	if llmAnswer == "" {
		global.Log.Warnf("[processMessageAsync] LLM返回空回复，转人工, 会话ID: %d", req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5, string(enum.ReplyMsgLlmEmpty))
		return
	}

	service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, llmAnswer)
	go d.updateConversationHistory(req.Conversation.ConversationID, req.Content, llmAnswer)
}

// getConversationHistory 获取会话历史记录，优先从Redis获取，否则从Chatwoot API获取
func (d *ChatApi) getConversationHistory(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error) {
	if global.RedisClient == nil {
		return nil, fmt.Errorf("Redis客户端未初始化")
	}

	// 1. 尝试从Redis获取聊天记录
	history, err := global.RedisClient.GetConversationHistory(ctx, conversationID)
	if err != nil {
		global.Log.Warnf("从Redis获取会话 %d 历史记录失败: %v, 将尝试从Chatwoot获取", conversationID, err)
	} else if history != nil {
		global.Log.Debugf("会话 %d 历史记录从Redis缓存命中", conversationID)
		// 缓存命中，直接返回历史记录。当前用户消息会作为LLM的content参数传入，无需在此处追加。
		return history, nil
	}

	if global.ChatwootService == nil {
		return nil, fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 2. 缓存未命中，从Chatwoot API获取完整的历史记录
	global.Log.Debugf("会话 %d 历史记录Redis缓存未命中，从Chatwoot API获取", conversationID)
	chatwootMessages, err := global.ChatwootService.GetConversationMessages(accountID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("从Chatwoot API获取会话 %d 消息失败: %w", conversationID, err)
	}

	// 3. 格式化历史记录为LLM需要的格式
	var formattedHistory []common.LlmMessage
	for _, msg := range chatwootMessages {
		// 过滤掉私信备注、没有内容的附件消息
		if msg.Private || msg.Content == "" {
			continue
		}

		// message_type为0代表incoming, 1代表outgoing
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

	// 4. 将格式化后的历史记录存入Redis，并设置过期时间 (异步操作)
	go func(history []common.LlmMessage, convID uint, ttl time.Duration) {
		if err := global.RedisClient.SetConversationHistory(context.Background(), convID, history, ttl); err != nil {
			global.Log.Errorf("异步将会话 %d 历史记录存入Redis失败: %v", convID, err)
		}
	}(formattedHistory, conversationID, time.Duration(global.Config.Redis.ConversationHistoryTTL)*time.Second)

	return formattedHistory, nil
}

// updateConversationHistory 在LLM回复后，更新Redis中的会话历史记录
func (d *ChatApi) updateConversationHistory(conversationID uint, userMessage, aiResponse string) {
	if global.RedisClient == nil {
		global.Log.Warnf("Redis客户端未初始化，无法更新会话 %d 历史记录", conversationID)
		return
	}

	ttl := time.Duration(global.Config.Redis.ConversationHistoryTTL) * time.Second

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

	ttl := time.Duration(global.Config.Redis.ConversationHistoryTTL) * time.Second
	messageToAppend := common.LlmMessage{
		Role:    "assistant",
		Content: req.Content,
	}

	if err := global.RedisClient.AppendToConversationHistory(context.Background(), req.Conversation.ConversationID, ttl, messageToAppend); err != nil {
		global.Log.Errorf("追加人工客服消息到会话 %d 历史记录失败: %v", req.Conversation.ConversationID, err)
	}
}
