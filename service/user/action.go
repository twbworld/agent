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

type ActionService interface {
	// 转接人工客服
	TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error
	// 将会话状态设置为机器人处理
	SetConversationPending(conversationID uint) error
	// 切换输入状态
	ToggleTyping(conversationID uint, status bool)
	// 发送消息
	SendMessage(conversationID uint, content string)
	// 匹配预设回复或执行特殊动作（如转人工）
	CannedResponses(chatRequest *common.ChatRequest) (string, bool, error)
}

type actionService struct {
	transferKeywords map[string]struct{}
}

// noGracePeriodReasons 定义了哪些转人工原因不需要设置宽限期，应立即转接
var noGracePeriodReasons = []enum.TransferToHuman{
	enum.TransferToHuman1,
	enum.TransferToHuman4,
	enum.TransferToHuman6,
}

func NewActionService() *actionService {
	// 初始化转人工的关键词列表,避免在每次调用时都创建map
	transferSet := make(map[string]struct{})
	keywordsList := global.Config.Ai.TransferKeywords
	for _, kw := range keywordsList {
		transferSet[strings.ToLower(kw)] = struct{}{}
	}

	return &actionService{
		transferKeywords: transferSet,
	}
}

func (d *actionService) TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 同步设置宽限期标志 (如果需要)
	gracePeriod := time.Duration(global.Config.Ai.TransferGracePeriod) * time.Second
	if gracePeriod > 0 && utils.InSlice(noGracePeriodReasons, remark) == -1 {
		if global.RedisClient != nil {
			key := fmt.Sprintf("%s%d", redis.KeyPrefixTransferGracePeriod, ConversationID)
			if err := global.RedisClient.Set(context.Background(), key, "1", gracePeriod).Err(); err != nil {
				global.Log.Warnf("[action]为会话 %d 设置转人工宽限期标志失败: %v", ConversationID, err)
			}
		}
	}

	g, _ := errgroup.WithContext(context.Background())

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
		if err := global.ChatwootService.SetConversationStatus(ConversationID, enum.ConversationStatusOpen); err != nil {
			global.Log.Errorf("[action]转接会话 %d 至人工客服失败: %v", ConversationID, err)
			return err
		}
		return nil
	})
	// 根据转接原因决定发送给用户的消息
	userMessage := ""
	if utils.InSlice(noGracePeriodReasons, remark) != -1 {
		// 显式转人工，或高优先级转人工
		userMessage = string(enum.ReplyMsgTransferSuccess)
	} else {
		// 隐式转人工，且有宽限期
		userMessage = string(enum.ReplyMsgAiRetrying)
	}

	// 如果有额外的消息参数，则覆盖默认消息
	if len(message) > 0 && message[0] != "" {
		userMessage = message[0]
	}

	if userMessage != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreateMessage(ConversationID, userMessage); err != nil {
				global.Log.Warnf("[action]为会话 %d 发送转人工提示失败: %v", ConversationID, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (d *actionService) SetConversationPending(conversationID uint) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}
	return global.ChatwootService.SetConversationStatus(conversationID, enum.ConversationStatusPending)
}

func (d *actionService) ToggleTyping(conversationID uint, status bool) {
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

func (d *actionService) SendMessage(conversationID uint, content string) {
	if global.ChatwootService == nil {
		return
	}
	if err := global.ChatwootService.CreateMessage(conversationID, content); err != nil {
		global.Log.Errorf("[action]向会话 %d 发送消息失败: %v", conversationID, err)
	}
}

// answer: 如果是普通回复，则为回复内容
// isAction: 如果匹配到特殊动作（如转人工），则为true
// err: 如果在匹配过程中发生错误
func (d *actionService) CannedResponses(chatRequest *common.ChatRequest) (string, bool, error) {
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
