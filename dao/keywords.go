package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	redisv8 "github.com/go-redis/redis/v8"
)

type KeywordsDb struct{}

// LoadAllKeywordsFromRedis 从Redis加载所有快捷回复
func (d *KeywordsDb) LoadAllKeywordsFromRedis(ctx context.Context) ([]chatwoot.CannedResponse, error) {
	if global.RedisClient == nil {
		return nil, errors.New("Redis客户端未初始化")
	}

	data, err := global.RedisClient.HGetAll(ctx, redis.KeyCannedResponsesHash).Result()
	if err != nil {
		return nil, fmt.Errorf("从Redis获取快捷回复失败: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	var responses []chatwoot.CannedResponse
	for _, v := range data {
		var resp chatwoot.CannedResponse
		if err := json.Unmarshal([]byte(v), &resp); err != nil {
			global.Log.Warnf("反序列化Redis中的快捷回复失败: %v, 数据: %s", err, v)
			continue
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

// SaveKeywordsToRedis 将快捷回复批量保存到Redis
// 使用HSet一次性设置多个字段，覆盖现有数据
func (d *KeywordsDb) SaveKeywordsToRedis(ctx context.Context, responses []chatwoot.CannedResponse) (int, error) {
	if global.RedisClient == nil {
		return 0, errors.New("Redis客户端未初始化")
	}
	if len(responses) == 0 {
		return 0, nil
	}

	// 使用HSet一次性设置多个字段
	// HSet key field1 value1 field2 value2 ...
	args := make([]interface{}, 0, len(responses)*2)
	for _, resp := range responses {
		// 检查 short_code 是否超出配置的长度限制
		if utf8.RuneCountInString(resp.ShortCode) > int(global.Config.Ai.MaxShortCodeLength) {
			global.Log.Warnf("short_code 超出长度限制，已跳过: %s", resp.ShortCode)
			continue
		}

		jsonBytes, err := json.Marshal(resp)
		if err != nil {
			global.Log.Warnf("序列化快捷回复失败: %v, 数据: %+v", err, resp)
			continue
		}
		args = append(args, resp.ShortCode, string(jsonBytes))
	}

	if len(args) == 0 {
		return 0, nil
	}

	err := global.RedisClient.HSet(ctx, redis.KeyCannedResponsesHash, args...).Err()
	if err != nil {
		return 0, fmt.Errorf("批量保存快捷回复到Redis失败: %w", err)
	}
	return len(responses), nil
}

// DeleteKeywordsFromRedis 从Redis删除指定的快捷回复
func (d *KeywordsDb) DeleteKeywordsFromRedis(ctx context.Context, shortCodes []string) (int, error) {
	if global.RedisClient == nil {
		return 0, errors.New("Redis客户端未初始化")
	}
	if len(shortCodes) == 0 {
		return 0, nil
	}

	count, err := global.RedisClient.HDel(ctx, redis.KeyCannedResponsesHash, shortCodes...).Result()
	if err != nil {
		return 0, fmt.Errorf("从Redis删除快捷回复失败: %w", err)
	}
	return int(count), nil
}

// AcquireSyncLock 尝试获取Redis分布式锁
func (d *KeywordsDb) AcquireSyncLock(ctx context.Context, agentID string) (bool, error) {
	if global.RedisClient == nil {
		return false, errors.New("Redis客户端未初始化")
	}
	expiry := time.Duration(global.Config.Redis.LockExpiry) * time.Second
	return global.RedisClient.SetNX(ctx, redis.KeySyncCannedResponsesLock, agentID, expiry).Result()
}

// ReleaseSyncLock 释放Redis分布式锁
func (d *KeywordsDb) ReleaseSyncLock(ctx context.Context, agentID string) error {
	if global.RedisClient == nil {
		return errors.New("Redis客户端未初始化")
	}

	// 确保只有持有锁的实例才能释放锁
	val, err := global.RedisClient.Get(ctx, redis.KeySyncCannedResponsesLock).Result()
	if err != nil && err != redisv8.Nil {
		return fmt.Errorf("获取锁值失败: %w", err)
	}
	if val == agentID {
		return global.RedisClient.Del(ctx, redis.KeySyncCannedResponsesLock).Err()
	}
	return nil // 不是当前实例持有的锁，无需释放
}
