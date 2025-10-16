package user

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
	"gitee.com/taoJie_1/chat/service"
)

type ChatApi struct{}

func (d *ChatApi) HandleChat(ctx *gin.Context) {
	var req common.ChatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Fail(ctx, "参数无效")
		return
	}

	// 调用验证器验证请求
	if err := service.Service.UserServiceGroup.Validator.ValidatorChatRequest(&req); err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	// 提示词长度校验
	if utf8.RuneCountInString(req.Content) > int(global.Config.Ai.MaxPromptLength) {
		global.Log.Warnf("用户 %d 提问内容过长，已转人工", req.Conversation.ConversationID)
		// 触发转人工
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman3)
		common.Fail(ctx, "提问内容过长，已为您转接人工客服")
		return
	}

	if req.MessageType != string(enum.MessageTypeIncoming) || req.Sender.Type != string(enum.SenderTypeContact) {
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
			global.Log.Errorf("[processMessageAsync]: %v", p)
			_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2)
		}
	}()

	// 关键词匹配和预设回复
	answer, isAction, err := service.Service.UserServiceGroup.ActionService.CannedResponses(&req)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] 匹配关键字错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2)
		return
	}
	if isAction {
		// 动作已执行（如转人工），流程结束
		return
	}
	if answer != "" {
		// 匹配到预设回复，直接发送消息
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, answer)
		return
	}

	// 向量数据库查询
	vectorResults, err := service.Service.UserServiceGroup.VectorService.Search(ctx, req.Content)
	if err != nil {
		// 只记录日志，不中断流程，因为后面还有LLM作为兜底
		global.Log.Errorf("[processMessageAsync] 向量数据库搜索错误: %v", err)
	}

	// 检查是否有足够相似的直接回复
	if len(vectorResults) > 0 && vectorResults[0].Similarity >= global.Config.Ai.VectorSimilarityThreshold {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, vectorResults[0].Answer)
		return
	}

	// 告诉用户“机器人正在输入中...”
	go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	// 在完成后,关闭“输入中”状态
	defer func() {
		go service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)
	}()

	// 准备给LLM的参考资料, 并通过context传递
	var llmReferenceDocs []dao.SearchResult
	if len(vectorResults) > 0 {
		for _, res := range vectorResults {
			if res.Similarity >= global.Config.Ai.VectorSearchMinSimilarity {
				llmReferenceDocs = append(llmReferenceDocs, res)
			}
		}
	}

	// 调用LLM服务获取回复, 显式传递参考资料
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.NewChat(ctx, &req, llmReferenceDocs)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] LLM错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2)
		return
	}

	if llmAnswer == "" {
		// 如果LLM没有返回任何内容，也可能是一个需要转人工的情况
		global.Log.Warnf("[processMessageAsync] LLM返回空回复 %d", req.Conversation.ConversationID)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5)
		return
	}

	service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, llmAnswer)
}
