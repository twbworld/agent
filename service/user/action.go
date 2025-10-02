package user

import (
	"strings"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
)

type ActionService struct {
	transferKeywords map[string]struct{}
}

func NewActionService() *ActionService {
	// 初始化转人工的关键词列表,避免在每次调用时都创建map
	transferSet := make(map[string]struct{})
	keywordsList := global.Config.Ai.TransferKeywords
	for _, kw := range keywordsList {
		transferSet[strings.ToLower(kw)] = struct{}{}
	}

	return &ActionService{
		transferKeywords: transferSet,
	}
}

// 转接人工客服
func (d *ActionService) TransferToHuman(ConversationID uint, remark enum.TransferToHuman) error {
	if remark != "" {
		// 先创建私信备注
		if err := global.ChatwootClient.CreatePrivateNote(ConversationID, string(remark)); err != nil {
			global.Log.Warnf("[action]为会话 %d 创建转人工备注失败: %v", ConversationID, err)
		}
	}

	// 将会话状态切换为 open
	if err := global.ChatwootClient.ToggleConversationStatus(ConversationID); err != nil {
		global.Log.Errorf("[action]转接会d %d 至人工客服失败: %v", ConversationID, err)
		return err
	}

	return nil
}

// 切换输入状态. status: true for 'on', false for 'off'
func (d *ActionService) ToggleTyping(conversationID uint, status bool) {
	statusStr := "off"
	if status {
		statusStr = "on"
	}
	// 这是一个非关键操作，即使失败也不应阻塞主流程，记录警告即可
	if err := global.ChatwootClient.ToggleTypingStatus(conversationID, statusStr); err != nil {
		global.Log.Warnf("[action]为会话 %d 切换typing状态失败: %v", conversationID, err)
	}
}

// 发送消息
func (d *ActionService) SendMessage(conversationID uint, content string) {
	if err := global.ChatwootClient.CreateMessage(conversationID, content); err != nil {
		// 这是一个关键操作，记录error级别日志
		global.Log.Errorf("[action]向会话 %d 发送消息失败: %v", conversationID, err)
	}
}

// 匹配预设回复或执行特殊动作（如转人工）
// 返回值: (answer string, isAction bool, err error)
// answer: 如果是普通回复，则为回复内容
// isAction: 如果执行了动作（如转人工），则为true，表示Controller无需再响应
// err: 如果执行动作时发生错误
func (d *ActionService) CannedResponses(chatRequest *common.ChatRequest) (string, bool, error) {
	content := strings.ToLower(strings.TrimSpace(chatRequest.Content))

	if content == "" {
		return "", false, nil
	}

	// 判断是否是"转人工"等关键字
	if _, isTransfer := d.transferKeywords[content]; isTransfer {
		//可后期判断多次关键字才转人工
		if err := d.TransferToHuman(chatRequest.Conversation.ConversationID, enum.TransferToHuman1); err != nil {
			return "", false, err
		}
		return "", true, nil
	}

	// 匹配"预设回复"的关键字
	global.CannedResponses.RLock()
	answer, ok := global.CannedResponses.Data[content]
	global.CannedResponses.RUnlock()

	if ok {
		return answer, false, nil
	}

	return "", false, nil
}
