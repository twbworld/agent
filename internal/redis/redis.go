package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitee.com/taoJie_1/mall-agent/model/common"
	"github.com/go-redis/redis/v8"
)

const (
	KeyCannedResponsesHash       = "canned_responses:hash"            // Redis中存储快捷回复的Hash Key
	KeySyncCannedResponsesLock   = "agent:lock:sync_canned_responses" // Redis分布式锁Key
	KeyPrefixConversationHistory = "conversation:history:"            // Redis中存储聊天记录的Key前缀
	KeyPrefixTransferGracePeriod = "agent:transfer_grace_period:"     // AI自动转人工后的宽限期Key前缀
)

// Service 定义了Redis操作的接口
type Service interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.StringStringMapCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Ping(ctx context.Context) *redis.StatusCmd
	GetConversationHistory(ctx context.Context, conversationID uint) ([]common.LlmMessage, error)
	SetConversationHistory(ctx context.Context, conversationID uint, history []common.LlmMessage, ttl time.Duration) error
	AppendToConversationHistory(ctx context.Context, conversationID uint, ttl time.Duration, newMessages ...common.LlmMessage) error
	Close() error
}

type client struct {
	rdb *redis.Client
}

// NewClient 创建一个新的Redis客户端实例
func NewClient(addr, password string, db int) (Service, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: 10, // 连接池大小
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("连接Redis失败: %w", err)
	}

	return &client{rdb: rdb}, nil
}

func (c *client) Close() error {
	return c.rdb.Close()
}

func (c *client) Get(ctx context.Context, key string) *redis.StringCmd {
	return c.rdb.Get(ctx, key)
}

func (c *client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	return c.rdb.Set(ctx, key, value, expiration)
}

func (c *client) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return c.rdb.Del(ctx, keys...)
}

func (c *client) HGetAll(ctx context.Context, key string) *redis.StringStringMapCmd {
	return c.rdb.HGetAll(ctx, key)
}

func (c *client) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return c.rdb.HSet(ctx, key, values...)
}

func (c *client) HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd {
	return c.rdb.HDel(ctx, key, fields...)
}

func (c *client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	return c.rdb.SetNX(ctx, key, value, expiration)
}

func (c *client) Ping(ctx context.Context) *redis.StatusCmd {
	return c.rdb.Ping(ctx)
}

// GetConversationHistory 从Redis获取指定会话的聊天记录
func (c *client) GetConversationHistory(ctx context.Context, conversationID uint) ([]common.LlmMessage, error) {
	key := fmt.Sprintf("%s%d", KeyPrefixConversationHistory, conversationID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // 缓存未命中
	}
	if err != nil {
		return nil, fmt.Errorf("从Redis获取聊天记录失败: %w", err)
	}

	var history []common.LlmMessage
	if err := json.Unmarshal([]byte(val), &history); err != nil {
		return nil, fmt.Errorf("反序列化聊天记录失败: %w", err)
	}
	return history, nil
}

// SetConversationHistory 将聊天记录保存到Redis，并设置过期时间
func (c *client) SetConversationHistory(ctx context.Context, conversationID uint, history []common.LlmMessage, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d", KeyPrefixConversationHistory, conversationID)
	jsonBytes, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("序列化聊天记录失败: %w", err)
	}
	return c.rdb.Set(ctx, key, jsonBytes, ttl).Err()
}

// AppendToConversationHistory 向Redis中指定会话的聊天记录追加一条或多条新消息，并重置过期时间
func (c *client) AppendToConversationHistory(ctx context.Context, conversationID uint, ttl time.Duration, newMessages ...common.LlmMessage) error {
	key := fmt.Sprintf("%s%d", KeyPrefixConversationHistory, conversationID)

	// 使用事务确保原子性
	err := c.rdb.Watch(ctx, func(tx *redis.Tx) error {
		val, err := tx.Get(ctx, key).Result()
		var history []common.LlmMessage
		if err == redis.Nil {
			history = []common.LlmMessage{}
		} else if err != nil {
			return fmt.Errorf("从Redis获取聊天记录失败: %w", err)
		} else {
			if err := json.Unmarshal([]byte(val), &history); err != nil {
				return fmt.Errorf("反序列化聊天记录失败: %w", err)
			}
		}

		history = append(history, newMessages...) // 追加多条消息
		jsonBytes, err := json.Marshal(history)
		if err != nil {
			return fmt.Errorf("序列化聊天记录失败: %w", err)
		}

		_, err = tx.Set(ctx, key, jsonBytes, ttl).Result()
		return err
	}, key)

	if err != nil {
		return fmt.Errorf("追加聊天记录到Redis失败: %w", err)
	}
	return nil
}
