package user

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	TransferToHuman(ctx context.Context, ConversationID uint, remark enum.TransferToHuman, message ...string) error
	// 将会话状态设置为机器人处理
	SetConversationPending(ctx context.Context, conversationID uint) error
	// 切换输入状态
	ToggleTyping(ctx context.Context, conversationID uint, status bool)
	// 发送消息
	SendMessage(ctx context.Context, conversationID uint, content string)
	// 匹配预设回复或执行特殊动作（如转人工）
	MatchCannedResponse(chatRequest *common.ChatRequest) (string, bool, error)
	// 设置人工模式宽限期
	ActivateHumanModeGracePeriod(ctx context.Context, conversationID uint)
	// 刷新人工模式宽限期
	RefreshHumanModeGracePeriod(ctx context.Context, conversationID uint)
	// 热更新转人工关键词
	UpdateTransferKeywords(keywords []string)
	// 根据规则分配会话给相应的团队
	AssignConversationByRules(ctx context.Context, conversationID uint, attrs common.CustomAttributes, currentTeamID *int)
}

type actionService struct {
	mu               sync.RWMutex
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
	a := &actionService{}
	a.UpdateTransferKeywords(global.Config.Ai.TransferKeywords)
	return a
}

func (a *actionService) UpdateTransferKeywords(keywords []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	newSet := make(map[string]struct{}, len(keywords))
	for _, kw := range keywords {
		newSet[strings.ToLower(kw)] = struct{}{}
	}
	a.transferKeywords = newSet
}

func (a *actionService) AssignConversationByRules(ctx context.Context, conversationID uint, attrs common.CustomAttributes, currentTeamID *int) {
	if global.ChatwootService == nil {
		return // 服务未初始化
	}

	preSalesID := global.Config.Chatwoot.Teams.PreSalesID
	afterSalesID := global.Config.Chatwoot.Teams.AfterSalesID
	defaultID := global.Config.Chatwoot.Teams.DefaultID

	var targetTeamID int

	if attrs.GoodsID != "" && preSalesID > 0 {
		targetTeamID = preSalesID
	} else if attrs.OrderID != "" && afterSalesID > 0 {
		targetTeamID = afterSalesID
	} else if defaultID > 0 {
		targetTeamID = defaultID
	}

	if targetTeamID == 0 {
		// 没有匹配到任何规则，并且没有配置默认团队，则不执行任何操作
		return
	}

	// 如果知道当前团队，并且与目标团队相同，则跳过以避免不必要的API调用
	if currentTeamID != nil && *currentTeamID == targetTeamID {
		global.Log.Debugf("会话 %d 已分配给目标团队 %d，跳过重复分配", conversationID, targetTeamID)
		return
	}

	if err := global.ChatwootService.AssignTeam(ctx, conversationID, targetTeamID); err != nil {
		global.Log.Warnf("为会话 %d 分配团队 %d 失败: %v", conversationID, targetTeamID, err)
	} else {
		global.Log.Debugf("已成功将会话 %d 分配给团队 %d", conversationID, targetTeamID)
	}
}

func (a *actionService) TransferToHuman(ctx context.Context, ConversationID uint, remark enum.TransferToHuman, message ...string) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 提前激活“人工模式宽限期”，确保在转为人工后，后续消息不会被AI立即接管
	a.ActivateHumanModeGracePeriod(context.Background(), ConversationID)

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

	// 确定提示消息内容
	userMessage := ""
	if utils.InSlice(noGracePeriodReasons, remark) != -1 {
		userMessage = string(enum.ReplyMsgTransferSuccess)
	} else {
		userMessage = string(enum.ReplyMsgAiRetrying)
	}

	if len(message) > 0 && message[0] != "" {
		userMessage = message[0]
	}

	// 第一阶段：并发执行“发送消息”和“创建备注”
	g, gCtx := errgroup.WithContext(ctx)
	if remark != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreatePrivateNote(gCtx, ConversationID, string(remark)); err != nil {
				global.Log.Warnf("[action]为会话 %d 创建转人工备注失败: %v", ConversationID, err)
			}
			return nil
		})
	}
	if userMessage != "" {
		g.Go(func() error {
			if err := global.ChatwootService.CreateMessage(gCtx, ConversationID, userMessage); err != nil {
				global.Log.Warnf("[action]为会话 %d 发送转人工提示失败: %v", ConversationID, err)
			}
			return nil
		})
	}
	_ = g.Wait()

	// 第二阶段：执行状态变更
	if err := global.ChatwootService.SetConversationStatus(ctx, ConversationID, chatwoot.ConversationStatusOpen); err != nil {
		global.Log.Errorf("[action]转接会话 %d 至人工客服失败: %v", ConversationID, err)
		return err
	}

	return nil
}

func (a *actionService) SetConversationPending(ctx context.Context, conversationID uint) error {
	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}
	return global.ChatwootService.SetConversationStatus(ctx, conversationID, chatwoot.ConversationStatusPending)
}

func (a *actionService) ToggleTyping(ctx context.Context, conversationID uint, status bool) {
	if global.ChatwootService == nil {
		return
	}
	statusStr := "off"
	if status {
		statusStr = "on"
	}
	if err := global.ChatwootService.ToggleTypingStatus(ctx, conversationID, statusStr); err != nil {
		global.Log.Warnf("[action]为会话 %d 切换typing状态失败: %v", conversationID, err)
	}
}

func (a *actionService) SendMessage(ctx context.Context, conversationID uint, content string) {
	if global.ChatwootService == nil {
		return
	}
	if err := global.ChatwootService.CreateMessage(ctx, conversationID, content); err != nil {
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

	// 判断是否是"转人工"等关键字 (加锁读取)
	a.mu.RLock()
	_, isTransfer := a.transferKeywords[content]
	a.mu.RUnlock()

	if isTransfer {
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

func (a *actionService) sendProductCard(ctx context.Context, conversationID uint, attrs common.CustomAttributes) {
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
	if err := global.ChatwootService.CreateCardMessage(ctx, conversationID, content, []chatwoot.CardItem{cardItem}); err != nil {
		global.Log.Errorf("[action]向会话 %d 发送商品卡片失败: %v", conversationID, err)
	}
}

// CheckAndSendProductCard 逻辑: 引入分布式锁防止并发下的重复发送
func (a *actionService) CheckAndSendProductCard(ctx context.Context, conversationID uint, attrs common.CustomAttributes) {
	// 快速检查：如果没有相关ID，直接返回，避免不必要的锁竞争
	if attrs.GoodsID == "" && attrs.OrderID == "" {
		return
	}

	if global.RedisClient == nil {
		return
	}

	// 1. 获取分布式锁
	// 目的：如果 WebWidgetTriggered 和 MessageCreated 同时触发，只有一个能获得锁执行检查
	lockKey := fmt.Sprintf("%s%d", redis.KeyPrefixProductCardLock, conversationID)
	// 尝试获取锁，设置5秒过期，防止死锁
	acquired, err := global.RedisClient.SetNX(ctx, lockKey, 1, 5*time.Second).Result()

	if err != nil {
		global.Log.Errorf("[CheckAndSendProductCard] Redis锁错误: %v", err)
		return
	}

	if !acquired {
		// 未获取到锁，说明已有协程在处理该会话的卡片发送逻辑，直接退出，避免重复
		global.Log.Debugf("会话 %d 正在发送卡片中，本次并发请求跳过", conversationID)
		return
	}
	// 确保逻辑结束后释放锁
	defer global.RedisClient.Del(ctx, lockKey)

	// 2. 执行原有的检查与发送逻辑（内部包含Redis去重判断）

	// --- 商品卡片逻辑 ---
	if attrs.GoodsID != "" {
		key := fmt.Sprintf("%s%d", redis.KeyPrefixLastProductSent, conversationID)

		// 获取该会话上次发送的商品ID
		lastSentGoodsID, err := global.RedisClient.Get(ctx, key).Result()
		if err != nil && err != redis.ErrNil {
			global.Log.Warnf("获取最后发送商品ID失败: %v", err)
		}

		// 再次检查Redis记录(Double Check)，防止在获取锁之前已有其他请求完成了发送
		if lastSentGoodsID != attrs.GoodsID {
			global.Log.Debugf("为会话 %d 发送商品 %s 的信息卡片 (上次: %s)", conversationID, attrs.GoodsID, lastSentGoodsID)

			// 发送卡片
			a.sendProductCard(ctx, conversationID, attrs)

			// 更新Redis记录，设置过期时间
			ttl := time.Duration(global.Config.Ai.ItemCardTTL) * time.Second
			if ttl > 0 {
				if err := global.RedisClient.Set(ctx, key, attrs.GoodsID, ttl).Err(); err != nil {
					global.Log.Warnf("更新会话 %d 最后发送商品ID失败: %v", conversationID, err)
				}
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
			ttl := time.Duration(global.Config.Ai.ItemCardTTL) * time.Second
			if ttl > 0 {
				if err := global.RedisClient.Set(ctx, key, attrs.OrderID, ttl).Err(); err != nil {
					global.Log.Warnf("更新会话 %d 最后发送订单ID失败: %v", conversationID, err)
				}
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
