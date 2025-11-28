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
	"gitee.com/taoJie_1/mall-agent/internal/mcp"
	"gitee.com/taoJie_1/mall-agent/internal/oss"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/internal/vector"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// setupLogFile 是一个辅助函数，用于创建和打开一个每日轮转的日志文件。
func (i *Initializer) setupLogFile(logPath string) (*os.File, error) {
	// 采用更通用的日志命名规范, 例如: gin.log -> gin.log.2025-10-28
	dateSuffix := time.Now().In(global.Tz).Format("2006-01-02")
	dailyLogPath := fmt.Sprintf("%s.%s", logPath, dateSuffix)

	if err := utils.CreateFile(dailyLogPath); err != nil {
		return nil, fmt.Errorf("创建日志文件 '%s' 失败: %w", dailyLogPath, err)
	}

	file, err := os.OpenFile(dailyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 '%s' 失败: %w", dailyLogPath, err)
	}

	i.logFileClosers = append(i.logFileClosers, file)
	return file, nil
}

// CustomJSONFormatter for logrus to set timezone
type CustomJSONFormatter struct {
	logrus.JSONFormatter
}

func (f *CustomJSONFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	entry.Time = entry.Time.In(global.Tz)
	return f.JSONFormatter.Format(entry)
}

// InitLog 初始化logrus日志库
func (i *Initializer) InitLog() error {
	runfile, err := i.setupLogFile(global.Config.RunLogPath)
	if err != nil {
		return fmt.Errorf("初始化运行日志失败: %w", err)
	}

	global.Log = logrus.New()
	global.Log.SetFormatter(&CustomJSONFormatter{
		JSONFormatter: logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "msg",
				logrus.FieldKeyTime:  "time",
			},
		},
	})
	if global.Config.Debug {
		global.Log.SetLevel(logrus.DebugLevel)
	} else {
		global.Log.SetLevel(logrus.InfoLevel)
	}

	global.Log.SetOutput(io.MultiWriter(os.Stdout, runfile))
	return nil
}

func (i *Initializer) InitTz() error {
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
		int(global.Config.Redis.DB),
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

func (i *Initializer) initMcp() error {
	if len(global.Config.McpServers) == 0 {
		return nil
	}
	client, err := mcp.NewClient(global.Log, global.Config.McpServers, global.Version, global.Config.ProjectName)
	if err != nil {
		global.Log.Warnf("MCP服务初始化失败: %v", err)
		return err
	}
	global.McpService = client
	global.Log.Info("初始化MCP服务结束")
	return nil
}

func (i *Initializer) mcpClose() error {
	if global.McpService != nil {
		return global.McpService.Close()
	}
	return nil
}

func (i *Initializer) initOss() error {
	cfg := global.Config.Oss
	// 检查OSS配置是否完整，使用新的 Bucket 字段名
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyId == "" || cfg.AccessKeySecret == "" {
		global.Log.Info("OSS配置不完整，跳过初始化")
		return nil
	}

	// 传递全局时区信息给OSS客户端
	client, err := oss.NewClient(cfg, global.Tz)
	if err != nil {
		global.Log.Warnf("初始化OSS服务失败: %v", err)
		return err
	}
	global.OssService = client
	global.Log.Info("初始化OSS服务成功")
	return nil
}

func (i *Initializer) ossClose() error {
	if global.OssService != nil {
		return global.OssService.Close()
	}
	return nil
}
