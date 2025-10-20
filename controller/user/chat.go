package user

import (
	"context"
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

	// 只处理来自用户的、消息类型为 "incoming" 的消息
	if req.MessageType != string(enum.MessageTypeIncoming) || req.Conversation.Meta.Sender.Type != string(enum.SenderTypeContact) {
		common.Success(ctx, nil)
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
		cannedAnswer  string
		isAction      bool
		vectorResults []dao.SearchResult
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
	// 等待快速路径搜索完成
	if err := g.Wait(); err != nil {
		// 如果是关键词匹配的错误，则转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2, string(enum.ReplyMsgSystemError))
		return
	}

	// 1. 检查是否触发了动作（如转人工）
	if isAction {
		return
	}
	// 2. 检查是否有精确匹配的答案
	if cannedAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, cannedAnswer)
		return
	}
	// 3. 检查是否有高相似度的向量搜索结果
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, vectorResults[0].Answer)
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
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.NewChat(ctx, &req, llmReferenceDocs)
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
}
