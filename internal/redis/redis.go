package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
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
