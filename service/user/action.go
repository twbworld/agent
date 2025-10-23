package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/utils"
	"golang.org/x/sync/errgroup"
)

type IActionService interface {
	TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error
	ToggleTyping(conversationID uint, status bool)
	SendMessage(conversationID uint, content string)
	CannedResponses(chatRequest *common.ChatRequest) (string, bool, error)
}

type ActionService struct {
	transferKeywords map[string]struct{}
}

// noGracePeriodReasons 定义了哪些转人工原因不需要设置宽限期，应立即转接
var noGracePeriodReasons = []enum.TransferToHuman{
	enum.TransferToHuman1, // 用户要求[转人工]
	enum.TransferToHuman4, // 用户情绪激动[转人工]
	enum.TransferToHuman6, // 金额过大[转人工]
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
func (d *ActionService) TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}
	g, _ := errgroup.WithContext(context.Background())

	// 如果转接原因不在“立即转接”列表中，则设置宽限期
	if utils.InSlice(noGracePeriodReasons, remark) == -1 {
		g.Go(func() error {
			if global.RedisClient != nil {
				const gracePeriod = 5 * time.Second
				key := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, ConversationID)
				if err := global.RedisClient.Set(context.Background(), key, "1", gracePeriod).Err(); err != nil {
					global.Log.Warnf("[action]为会话 %d 设置转人工宽限期标志失败: %v", ConversationID, err)
				}
			}
			return nil
		})
	}

	// 创建私信备注（内部使用）
	if remark != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreatePrivateNote(ConversationID, string(remark)); err != nil {
				global.Log.Warnf("[action]为会话 %d 创建转人工备注失败: %v", ConversationID, err)
			}
			return nil
		})
	}

	g.Go(func() error {
		if err := global.ChatwootService.ToggleConversationStatus(ConversationID); err != nil {
			global.Log.Errorf("[action]转接会话 %d 至人工客服失败: %v", ConversationID, err)
			return err
		}
		return nil
	})

	if len(message) > 0 && message[0] != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreateMessage(ConversationID, message[0]); err != nil {
				global.Log.Warnf("[action]为会话 %d 发送转人工提示失败: %v", ConversationID, err)
			}
			return nil
		})
	}

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
// isAction: 如果匹配到特殊动作（如转人工），则为true
// err: 如果在匹配过程中发生错误
func (d *ActionService) CannedResponses(chatRequest *common.ChatRequest) (string, bool, error) {
	content := strings.ToLower(strings.TrimSpace(chatRequest.Content))

	if content == "" {
		return "", false, nil
	}

	// 判断是否是"转人工"等关键字
	if _, isTransfer := d.transferKeywords[content]; isTransfer {
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
