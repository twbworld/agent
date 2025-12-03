package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/utils"
	"golang.org/x/sync/errgroup"
)

type ActionService interface {
	// 检查并发送商品/订单卡片
	CheckAndSendProductCard(ctx context.Context, conversationID uint, attrs common.CustomAttributes)
	// 转接人工客服
	TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error
	// 将会话状态设置为机器人处理
	SetConversationPending(conversationID uint) error
	// 切换输入状态
	ToggleTyping(conversationID uint, status bool)
	// 发送消息
	SendMessage(conversationID uint, content string)
	// 匹配预设回复或执行特殊动作（如转人工）
	MatchCannedResponse(chatRequest *common.ChatRequest) (string, bool, error)
	// 设置人工模式宽限期
	ActivateHumanModeGracePeriod(ctx context.Context, conversationID uint)
	// 刷新人工模式宽限期
	RefreshHumanModeGracePeriod(ctx context.Context, conversationID uint)
}

type actionService struct {
	transferKeywords map[string]struct{}
}

// noGracePeriodReasons 定义了哪些转人工原因不需要设置宽限期，应立即转接
var noGracePeriodReasons = []enum.TransferToHuman{
	enum.TransferToHuman1,
	enum.TransferToHuman4,
	enum.TransferToHuman6,
	enum.TransferToHuman5,
}

func NewActionService() ActionService {
	// 初始化转人工的关键词列表
	transferSet := make(map[string]struct{})
	keywordsList := global.Config.Ai.TransferKeywords
	for _, kw := range keywordsList {
		transferSet[strings.ToLower(kw)] = struct{}{}
	}

	return &actionService{
		transferKeywords: transferSet,
	}
}

func (a *actionService) TransferToHuman(ConversationID uint, remark enum.TransferToHuman, message ...string) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 同步设置宽限期标志
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

	// 创建私信备注
	if remark != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreatePrivateNote(ConversationID, string(remark)); err != nil {
				global.Log.Warnf("[action]为会话 %d 创建转人工备注失败: %v", ConversationID, err)
			}
			return nil
		})
	}
	g.Go(func() error {
		if err := global.ChatwootService.SetConversationStatus(ConversationID, chatwoot.ConversationStatusOpen); err != nil {
			global.Log.Errorf("[action]转接会话 %d 至人工客服失败: %v", ConversationID, err)
			return err
		}
		return nil
	})

	userMessage := ""
	if utils.InSlice(noGracePeriodReasons, remark) != -1 {
		userMessage = string(enum.ReplyMsgTransferSuccess)
	} else {
		userMessage = string(enum.ReplyMsgAiRetrying)
	}

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

	return g.Wait()
}

func (a *actionService) SetConversationPending(conversationID uint) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}
	return global.ChatwootService.SetConversationStatus(conversationID, chatwoot.ConversationStatusPending)
}

func (a *actionService) ToggleTyping(conversationID uint, status bool) {
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

func (a *actionService) SendMessage(conversationID uint, content string) {
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
func (a *actionService) MatchCannedResponse(chatRequest *common.ChatRequest) (string, bool, error) {
	content := strings.ToLower(strings.TrimSpace(chatRequest.Content))
	if content == "" {
		return "", false, nil
	}

	// 判断是否是"转人工"等关键字
	if _, isTransfer := a.transferKeywords[content]; isTransfer {
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

func (a *actionService) sendProductCard(conversationID uint, attrs common.CustomAttributes) {
	if global.ChatwootService == nil {
		global.Log.Warnf("[action] Chatwoot客户端未初始化，无法为会话 %d 发送商品卡片", conversationID)
		return
	}

	cardItem := chatwoot.CardItem{
		MediaURL:    attrs.GoodsImage,
		Title:       attrs.GoodsTitle,
		Description: "您正在咨询的商品",
		Actions: []chatwoot.CardAction{
			{
				Type: "link",
				Text: "查看详情",
				URI:  attrs.GoodsUrl,
			},
		},
	}
	content := fmt.Sprintf("商品名称：**%s**\n价格：%s\ngoods_id：%s\n[![%s](%s)](%s)", attrs.GoodsTitle, attrs.GoodsPrice, attrs.GoodsID, attrs.GoodsTitle, attrs.GoodsImage, attrs.GoodsUrl)

	//(当前消息不加入缓存)
	if err := global.ChatwootService.CreateCardMessage(conversationID, content, []chatwoot.CardItem{cardItem}); err != nil {
		global.Log.Errorf("[action]向会话 %d 发送商品卡片失败: %v", conversationID, err)
	}
}

// CheckAndSendProductCard 逻辑: 记录上次发送的ID，如果当前咨询的ID与上次不同，则发送卡片。
func (a *actionService) CheckAndSendProductCard(ctx context.Context, conversationID uint, attrs common.CustomAttributes) {
	// --- 商品卡片逻辑 ---
	if attrs.GoodsID != "" {
		key := fmt.Sprintf("%s%d", redis.KeyPrefixLastProductSent, conversationID)

		// 获取该会话上次发送的商品ID
		lastSentGoodsID, err := global.RedisClient.Get(ctx, key).Result()
		if err != nil && err != redis.ErrNil {
			global.Log.Warnf("获取最后发送商品ID失败: %v", err)
		}

		// 如果Redis中没有记录(首次)，或者记录的ID与当前ID不一致，则发送
		if lastSentGoodsID != attrs.GoodsID {
			global.Log.Debugf("为会话 %d 发送商品 %s 的信息卡片 (上次: %s)", conversationID, attrs.GoodsID, lastSentGoodsID)

			// 发送卡片
			a.sendProductCard(conversationID, attrs)

			// 更新Redis记录，设置24小时过期
			if err := global.RedisClient.Set(ctx, key, attrs.GoodsID, 24*time.Hour).Err(); err != nil {
				global.Log.Warnf("更新会话 %d 最后发送商品ID失败: %v", conversationID, err)
			}
		}
	}

	// --- 订单卡片逻辑 (预留) ---
	if attrs.OrderID != "" {
		key := fmt.Sprintf("%s%d", redis.KeyPrefixLastOrderSent, conversationID)

		lastSentOrderID, err := global.RedisClient.Get(ctx, key).Result()
		if err != nil && err != redis.ErrNil {
			global.Log.Warnf("获取最后发送订单ID失败: %v", err)
		}

		if lastSentOrderID != attrs.OrderID {
			global.Log.Debugf("为会话 %d 发送订单 %s 的信息卡片 (上次: %s)", conversationID, attrs.OrderID, lastSentOrderID)
			// a.sendOrderCard(conversationID, attrs) // 预留
			if err := global.RedisClient.Set(ctx, key, attrs.OrderID, 24*time.Hour).Err(); err != nil {
				global.Log.Warnf("更新会话 %d 最后发送订单ID失败: %v", conversationID, err)
			}
		}
	}
}

func (a *actionService) ActivateHumanModeGracePeriod(ctx context.Context, conversationID uint) {
	if global.Config.Ai.HumanModeGracePeriod <= 0 {
		return
	}
	key := fmt.Sprintf("%s%d", redis.KeyPrefixHumanModeActive, conversationID)
	ttl := time.Duration(global.Config.Ai.HumanModeGracePeriod) * time.Second
	err := global.RedisClient.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		global.Log.Warnf("为会话 %d 设置人工模式宽限期失败: %v", conversationID, err)
	}
}

func (a *actionService) RefreshHumanModeGracePeriod(ctx context.Context, conversationID uint) {
	if global.Config.Ai.HumanModeGracePeriod <= 0 {
		return
	}
	key := fmt.Sprintf("%s%d", redis.KeyPrefixHumanModeActive, conversationID)
	ttl := time.Duration(global.Config.Ai.HumanModeGracePeriod) * time.Second
	// 使用 Expire 刷新过期时间，仅当 key 存在时生效
	updated, err := global.RedisClient.Expire(ctx, key, ttl).Result()
	if err != nil {
		global.Log.Warnf("刷新会话 %d 人工模式宽限期失败: %v", conversationID, err)
	} else if updated {
		global.Log.Debugf("收到用户消息，已刷新会话 %d 的人工模式宽限期", conversationID)
	}
}
