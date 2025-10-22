package initialize

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/embedding"
	"gitee.com/taoJie_1/mall-agent/internal/llm"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/internal/vector"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// InitLog 初始化logrus日志库
func (i *Initializer) InitLog() error {
	if err := utils.CreateFile(global.Config.RunLogPath); err != nil {
		return fmt.Errorf("创建文件错误[oirdtug]: %w", err)
	}

	global.Log = logrus.New()
	global.Log.SetFormatter(&logrus.JSONFormatter{})
	if gin.Mode() == gin.DebugMode {
		global.Log.SetLevel(logrus.DebugLevel)
	} else {
		global.Log.SetLevel(logrus.InfoLevel)
	}

	runfile, err := os.OpenFile(global.Config.RunLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开文件错误[0atrpf]: %w", err)
	}
	global.Log.SetOutput(io.MultiWriter(os.Stdout, runfile))
	i.logFileCloser = runfile // 存储文件关闭器
	return nil
}

func (i *Initializer) initTz() error {
	Location, err := time.LoadLocation(global.Config.Tz)
	if err != nil {
		return fmt.Errorf("时区配置失败[siortuj]: %w", err)
	}
	global.Tz = Location
	return nil
}

// initRedis 初始化Redis客户端
func (i *Initializer) initRedis() error {
	client, err := redis.NewClient(
		global.Config.Redis.Addr,
		global.Config.Redis.Password,
		global.Config.Redis.DB,
	)
	if err != nil {
		return fmt.Errorf("初始化Redis客户端失败: %w", err)
	}
	global.RedisClient = client
	global.Log.Info("初始化Redis服务成功")
	return nil
}

// redisClose 关闭Redis客户端连接
func (i *Initializer) redisClose() error {
	if global.RedisClient != nil {
		return global.RedisClient.Close()
	}
	return nil
}

func (i *Initializer) initVectorDb() error {
	client, err := vector.NewClient(
		global.Config.VectorDb.Url,
		global.Config.VectorDb.Auth,
	)
	if err != nil {
		global.Log.Warnf("创建VectorDb客户端失败: %v", err)
		return err
	}

	// 通过心跳检测验证与VectorDb服务的连接
	err = client.Heartbeat(context.Background())
	if err != nil {
		global.Log.Warnf("无法连接到VectorDb服务 (url: %s): %v", global.Config.VectorDb.Url, err)
		return err
	}

	global.VectorDb = client
	dao.App.VectorDb.CollectionName = global.Config.VectorDb.CollectionName
	global.Log.Info("初始化VectorDb服务成功")
	return nil
}

// vectorDbClose 关闭VectorDb客户端连接
func (i *Initializer) vectorDbClose() error {
	if global.VectorDb != nil {
		return global.VectorDb.Close()
	}
	return nil
}

func (i *Initializer) initChatwoot() error {
	client := chatwoot.NewClient(
		global.Config.Chatwoot.Url,
		int(global.Config.Chatwoot.AccountId),
		global.Config.Chatwoot.Auth,
		global.Config.Chatwoot.BotAuth,
		global.Log,
	)

	if _, err := client.GetAccountDetails(); err != nil {
		return fmt.Errorf("无法连接到Chatwoot服务 (url: %s, token: AgentApiToken): %w", global.Config.Chatwoot.Url, err)
	}

	global.ChatwootService = client
	global.Log.Info("初始化Chatwoot服务成功")
	return nil
}

func (i *Initializer) initLlm() error {
	if err := i.doInitLlm(); err != nil {
		global.Log.Warnf("初始化LLM服务失败: %v", err)
		return err
	}
	global.Log.Info("初始化LLM服务成功")
	return nil
}

func (i *Initializer) doInitLlm() error {
	if len(global.Config.Llm) == 0 {
		return fmt.Errorf("未配置任何LLM")
	}

	llmClients := make(map[enum.LlmSize]*openai.Client, len(global.Config.Llm))
	for _, cfg := range global.Config.Llm {
		config := openai.DefaultConfig(cfg.Auth)
		config.BaseURL = cfg.Url
		config.HTTPClient = &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second}
		llmClients[enum.LlmSize(cfg.Size)] = openai.NewClientWithConfig(config)
	}

	g, gCtx := errgroup.WithContext(context.Background())
	// 并发地对所有配置的LLM服务进行连接测试
	for _, cfg := range global.Config.Llm {
		cfg := cfg // 避免闭包陷阱
		g.Go(func() error {
			size := enum.LlmSize(cfg.Size)
			client := llmClients[size]

			// 使用 errgroup 的 context，以便在任何一个 goroutine 失败时可以取消其他的
			reqCtx, cancel := context.WithTimeout(gCtx, 5*time.Second)
			defer cancel()

			// 通过ListModels接口验证服务是否可用
			if _, err := client.ListModels(reqCtx); err != nil {
				return fmt.Errorf("无法连接到LLM服务 (size: %s, url: %s): %w", size, cfg.Url, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	global.LlmService = llm.NewClient(
		global.Log,
		llmClients,
		global.Config.Llm,
	)
	return nil
}

func (i *Initializer) initLlmEmbedding() error {
	if err := i.doInitLlmEmbedding(); err != nil {
		global.Log.Warnf("初始化向量化服务失败: %v", err)
		return err
	}
	global.Log.Info("初始化向量化服务成功")
	return nil
}

func (i *Initializer) doInitLlmEmbedding() error {
	config := openai.DefaultConfig(global.Config.LlmEmbedding.Auth)
	config.BaseURL = global.Config.LlmEmbedding.Url
	openAIClient := openai.NewClientWithConfig(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 通过ListModels接口验证向量化服务是否可用
	if _, err := openAIClient.ListModels(ctx); err != nil {
		return fmt.Errorf("无法连接到向量化服务 (url: %s): %w", config.BaseURL, err)
	}

	global.EmbeddingService = embedding.NewClient(
		openAIClient,
		global.Config.LlmEmbedding.Model,
	)
	return nil
}
