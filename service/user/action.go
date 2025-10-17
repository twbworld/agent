package user

import (
	"context"
	"fmt"
	"strings"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"golang.org/x/sync/errgroup"
)

type IActionService interface {
	TransferToHuman(ConversationID uint, remark enum.TransferToHuman) error
	ToggleTyping(conversationID uint, status bool)
	SendMessage(conversationID uint, content string)
	CannedResponses(chatRequest *common.ChatRequest) (string, bool, error)
}

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
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}
	g, _ := errgroup.WithContext(context.Background())

	// 创建私信备注
	if remark != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreatePrivateNote(ConversationID, string(remark)); err != nil {
				global.Log.Warnf("[action]为会话 %d 创建转人工备注失败: %v", ConversationID, err)
			}
			return nil
		})
	}

	// 切换会话状态
	g.Go(func() error {
		if err := global.ChatwootService.ToggleConversationStatus(ConversationID); err != nil {
			global.Log.Errorf("[action]转接会话 %d 至人工客服失败: %v", ConversationID, err)
			return err
		}
		return nil
	})

	// 等待所有任务完成，并返回遇到的第一个错误
	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

// 切换输入状态
func (d *ActionService) ToggleTyping(conversationID uint, status bool) {
	if global.ChatwootService == nil {
		return
	}
	statusStr := "off"
	if status {
		statusStr = "on"
	}
	if err := global.ChatwootService.ToggleTypingStatus(conversationID, statusStr); err != nil {
		global.Log.Warnf("[action]为会话 %d 切换typing状态失败: %v", conversationID, err)
	}
}

// 发送消息
func (d *ActionService) SendMessage(conversationID uint, content string) {
	if global.ChatwootService == nil {
		return
	}
	if err := global.ChatwootService.CreateMessage(conversationID, content); err != nil {
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
		//(可后期判断多次关键字才转人工)
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
