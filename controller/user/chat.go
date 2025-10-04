package user

import (
	"context"
	"github.com/gin-gonic/gin"
	"unicode/utf8"

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
	// 将原始请求的上下文传递给异步处理函数，以便在用户关闭浏览器时取消LLM请求
	go d.processMessageAsync(ctx.Request.Context(), reqCopy)
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

	//....这里缺少查向量数据库的逻辑?并把向量详细传给LLM?

	// 告诉用户“机器人正在输入中...”
	service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, true)
	// 在完成后,关闭“输入中”状态
	defer service.Service.UserServiceGroup.ActionService.ToggleTyping(req.Conversation.ConversationID, false)

	// 调用LLM服务获取回复
	llmAnswer, err := service.Service.UserServiceGroup.LlmService.NewChat(ctx, &req)
	if err != nil {
		global.Log.Errorf("[processMessageAsync] LLM错误: %v", err)
		_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman2)
		return
	}

	// 确保LLM有返回内容
	if llmAnswer != "" {
		service.Service.UserServiceGroup.ActionService.SendMessage(req.Conversation.ConversationID, llmAnswer)
		return
	}

	// 如果LLM没有返回任何内容，也可能是一个需要转人工的情况
	global.Log.Warnf("[processMessageAsync] LLM返回空回复 %d", req.Conversation.ConversationID)
	_ = service.Service.UserServiceGroup.ActionService.TransferToHuman(req.Conversation.ConversationID, enum.TransferToHuman5)
}
