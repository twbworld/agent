package user

import (
	"context"
	"fmt"
	"os"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/sashabaranov/go-openai"
)

// HistoryService 定义了会话历史缓存服务的接口
type HistoryService interface {
	// GetOrFetch 封装了完整的“缓存优先”逻辑。
	// 它首先尝试从Redis获取历史记录。如果缓存未命中，它将使用分布式锁来防止缓存击穿，
	// 然后从Chatwoot API回源获取数据，最后将数据存入Redis并返回。
	GetOrFetch(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error)

	// Append 将一条或多条消息原子性地追加到指定会话的历史记录中，并刷新其TTL。
	Append(ctx context.Context, conversationID uint, messages ...common.LlmMessage) error

	// Set 直接用给定的历史记录覆盖Redis中的缓存。
	Set(ctx context.Context, conversationID uint, history []common.LlmMessage) error
}

type historyService struct{}

// NewHistoryService 创建一个新的 HistoryService 实例
func NewHistoryService() HistoryService {
	return &historyService{}
}

func (s *historyService) GetOrFetch(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error) {
	if global.RedisClient == nil {
		return nil, fmt.Errorf("Redis客户端未初始化")
	}

	// 1. 尝试从Redis获取聊天记录
	history, err := global.RedisClient.GetConversationHistory(ctx, conversationID)
	if err != nil && err != redis.ErrNil { // Redis error other than miss
		global.Log.Warnf("从Redis获取会话 %d 历史记录失败: %v, 将尝试从Chatwoot获取", conversationID, err)
	} else if history != nil { // Cache hit
		global.Log.Debugf("会话 %d 历史记录从Redis缓存命中", conversationID)
		return history, nil
	}

	// --- 缓存未命中，进入回源逻辑 ---

	if global.ChatwootService == nil {
		return nil, fmt.Errorf("Chatwoot客户端未初始化")
	}

	// 2. 使用分布式锁防止缓存击穿
	lockKey := fmt.Sprintf("%s%d", redis.KeyPrefixHistoryLock, conversationID)
	lockExpiry := time.Duration(global.Config.Redis.HistoryLockExpiry) * time.Second
	agentID, _ := os.Hostname()
	if agentID == "" {
		agentID = "unknown-agent"
	}

	locked, err := global.RedisClient.SetNX(ctx, lockKey, agentID, lockExpiry).Result()
	if err != nil {
		global.Log.Errorf("尝试获取会话 %d 历史记录锁失败: %v", conversationID, err)
		// 即使获取锁失败，也尝试从源获取，作为降级策略
		return s.fetchAndCache(ctx, accountID, conversationID, currentMessage)
	}

	if locked {
		// 2a. 成功获取锁，从Chatwoot API获取数据并缓存
		global.Log.Debugf("会话 %d 历史记录Redis缓存未命中，成功获取锁，从Chatwoot API获取", conversationID)
		defer func() {
			// 使用后台 context 确保即使原始请求取消，锁释放也能执行
			if err := global.RedisClient.Del(context.Background(), lockKey).Err(); err != nil {
				global.Log.Warnf("释放会话 %d 历史记录锁失败: %v", conversationID, err)
			}
		}()
		// 在获取锁后，再次检查缓存，防止在获取锁的过程中，已有其他请求完成了缓存填充（双重检查锁定）
		history, err := global.RedisClient.GetConversationHistory(ctx, conversationID)
		if err == nil && history != nil {
			global.Log.Debugf("获取锁后发现会话 %d 缓存已存在", conversationID)
			return history, nil
		}
		return s.fetchAndCache(ctx, accountID, conversationID, currentMessage)
	}

	// 2b. 未获取到锁，说明其他goroutine正在回源，等待后重试
	global.Log.Debugf("会话 %d 历史记录锁被占用，等待后重试", conversationID)
	time.Sleep(200 * time.Millisecond) // 短暂等待

	history, err = global.RedisClient.GetConversationHistory(ctx, conversationID)
	if err == nil && history != nil {
		global.Log.Debugf("等待后，会话 %d 历史记录从Redis缓存命中", conversationID)
		return history, nil
	}

	// 如果等待后仍然没有缓存，作为降级策略，直接从源获取数据
	global.Log.Warnf("等待后会话 %d 缓存仍未命中，直接回源作为降级策略", conversationID)
	return s.fetchAndCache(ctx, accountID, conversationID, currentMessage)
}

func (s *historyService) Append(ctx context.Context, conversationID uint, messages ...common.LlmMessage) error {
	if global.RedisClient == nil {
		return fmt.Errorf("Redis客户端未初始化")
	}
	if len(messages) == 0 {
		return nil
	}

	ttl := utils.GetTTLWithJitter(global.Config.Redis.ConversationHistoryTTL)
	err := global.RedisClient.AppendToConversationHistory(ctx, conversationID, ttl, messages...)
	if err != nil {
		global.Log.Errorf("追加消息到会话 %d 历史记录失败: %v", conversationID, err)
	}
	return err
}

func (s *historyService) Set(ctx context.Context, conversationID uint, history []common.LlmMessage) error {
	if global.RedisClient == nil {
		return fmt.Errorf("Redis客户端未初始化")
	}
	ttl := utils.GetTTLWithJitter(global.Config.Redis.ConversationHistoryTTL)
	err := global.RedisClient.SetConversationHistory(ctx, conversationID, history, ttl)
	if err != nil {
		global.Log.Errorf("设置会话 %d 历史记录失败: %v", conversationID, err)
	}
	return err
}

// fetchAndCache 是一个私有辅助方法，用于从Chatwoot获取数据、格式化并存入Redis
func (s *historyService) fetchAndCache(ctx context.Context, accountID, conversationID uint, currentMessage string) ([]common.LlmMessage, error) {
	// 从Chatwoot API获取完整的历史记录
	chatwootMessages, err := global.ChatwootService.GetConversationMessages(accountID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("从Chatwoot API获取会话 %d 消息失败: %w", conversationID, err)
	}

	// 格式化历史记录为LLM需要的格式
	var formattedHistory []common.LlmMessage
	for _, msg := range chatwootMessages {
		// 过滤掉私信备注、没有内容的附件消息
		if msg.Private || msg.Content == "" {
			continue
		}

		// 过滤掉当前用户消息，因为它会作为LLM的content参数传入，避免重复
		if msg.MessageType == chatwoot.MessageDirectionIncoming && msg.Sender.Type == chatwoot.SenderContact && msg.Content == currentMessage {
			continue
		}

		var role string
		if msg.MessageType == chatwoot.MessageDirectionIncoming && msg.Sender.Type == chatwoot.SenderContact {
			role = openai.ChatMessageRoleUser
		} else if msg.MessageType == chatwoot.MessageDirectionOutgoing {
			role = openai.ChatMessageRoleAssistant // 假设所有outgoing消息都是AI或客服的回复
		} else {
			continue // 忽略其他类型的消息
		}
		formattedHistory = append(formattedHistory, common.LlmMessage{Role: role, Content: msg.Content})
	}

	// 将格式化后的历史记录存入Redis
	if err := s.Set(context.Background(), conversationID, formattedHistory); err != nil {
		// 只记录错误，不阻塞返回
		global.Log.Errorf("将会话 %d 历史记录存入Redis失败: %v", conversationID, err)
	}

	return formattedHistory, nil
}
