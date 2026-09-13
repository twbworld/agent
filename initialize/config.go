package initialize

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/model/config"
	"github.com/twbworld/agent/service"
	"github.com/twbworld/agent/task"
	"golang.org/x/sync/errgroup"
)

var (
	Conf string
	Act  string
)

func init() {
	flag.StringVar(&Conf, "c", "", "choose config file.")
	flag.StringVar(&Act, "a", "", `行为,默认为空,即启动服务; "clear": 清除过期数据;`)
}

// New 创建一个新的初始化器，并加载配置文件
func New(taskManager *task.Manager) *Initializer {
	var configPath string
	if gin.Mode() != gin.TestMode {
		flag.Parse()
		if Conf != "" {
			configPath = Conf
		}
	}
	if configPath == "" {
		configPath = `config.yaml`
	}

	// 将 Initializer 实例创建提前
	i := &Initializer{taskManager: taskManager}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		panic("读取配置失败[u9ij]: " + configPath + err.Error())
	}

	// --- 为热重载引入Debounce(防抖)机制 ---
	var (
		debounceTimer *time.Timer
		debounceMutex sync.Mutex
	)
	const debounceDuration = 500 * time.Millisecond // 增加防抖时间，确保文件写入完成

	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		debounceMutex.Lock()
		defer debounceMutex.Unlock()

		// 如果已有计时器，则重置它
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		// 启动一个新的计时器，在持续时间内如果没有新事件，则执行重载
		debounceTimer = time.AfterFunc(debounceDuration, func() {
			fmt.Println("配置文件变化[djiads] (Debounced): ", e.Name)

			// 读锁
			global.ConfigLock.RLock()
			oldConfig := global.Config.DeepCopy()
			global.ConfigLock.RUnlock()

			// 反序列化到临时对象，避免直接污染全局变量
			tmpConfig := &config.Config{}
			if err := v.Unmarshal(tmpConfig); err != nil {
				fmt.Println("热加载配置文件反序列化失败:", err)
				return
			}

			handleConfig(tmpConfig)

			// 安全替换全局配置 (写锁)
			global.ConfigLock.Lock()
			// 替换指针的内容;注意: 这里不能直接 global.Config = tmpConfig，因为其他地方可能持有了旧指针
			*global.Config = *tmpConfig
			global.ConfigLock.Unlock()

			// 调用新的处理函数来处理配置变更
			i.HandleConfigChange(oldConfig, global.Config)
		})
	})

	if err := v.Unmarshal(global.Config); err != nil {
		panic("出错[dhfal]: " + err.Error())
	}

	handleConfig(global.Config)

	return i
}

// handleConfig 处理和设置配置的默认值
func handleConfig(c *config.Config) {
	c.StaticDir = strings.TrimRight(c.StaticDir, "/")

	if c.ProjectName == "" {
		c.ProjectName = "mall-Agent"
	}
	if c.GinAddr == "" {
		c.GinAddr = ":80"
	}
	if c.StaticDir == "" {
		c.StaticDir = "static"
	}
	if c.GinLogPath == "" {
		c.GinLogPath = "log/gin.log"
	}
	if c.RunLogPath == "" {
		c.RunLogPath = "log/run.log"
	}
	if c.LogRetentionDays == 0 {
		c.LogRetentionDays = 7
	}
	if c.Tz == "" {
		c.Tz = "Asia/Shanghai"
	}
	if len(c.Cors) == 0 {
		c.Cors = []string{"*"}
	}
	if c.Database.Type == "" {
		c.Database.Type = "sqlite"
	}
	if c.Database.SqlitePath == "" {
		c.Database.SqlitePath = "data.db"
	}
	if c.Redis.Addr == "" {
		c.Redis.Addr = "127.0.0.1:6379"
	}
	if c.Redis.Password == "" {
		c.Redis.Password = ""
	}
	if c.Redis.DB == 0 {
		c.Redis.DB = 0
	}
	if c.Redis.LockExpiry == 0 {
		c.Redis.LockExpiry = 30
	}
	if c.Redis.ConversationHistoryTTL == 0 {
		c.Redis.ConversationHistoryTTL = 3600
	}
	if c.Redis.HistoryLockExpiry == 0 {
		c.Redis.HistoryLockExpiry = 10
	}
	if c.Chatwoot.Url == "" {
		c.Chatwoot.Url = "http://127.0.0.1:3000"
	}
	if c.Chatwoot.AccountId == 0 {
		c.Chatwoot.AccountId = 1
	}
	for i := range c.Llm {
		if c.Llm[i].Timeout == 0 {
			c.Llm[i].Timeout = 60
		}
	}
	if c.LlmEmbedding.Timeout == 0 {
		c.LlmEmbedding.Timeout = 5
	}
	if c.LlmEmbedding.BatchTimeout == 0 {
		c.LlmEmbedding.BatchTimeout = 60
	}
	if c.VectorDb.CollectionName == "" {
		c.VectorDb.CollectionName = "chatwoot_keywords"
	}
	if c.Ai.MaxPromptLength == 0 {
		c.Ai.MaxPromptLength = 1000
	}
	if c.Ai.MaxShortCodeLength == 0 {
		c.Ai.MaxShortCodeLength = 255
	}
	if c.Ai.SemanticPrefix == "" {
		c.Ai.SemanticPrefix = "ai@"
	}
	if c.Ai.HybridPrefix == "" {
		c.Ai.HybridPrefix = "ai+@"
	}
	if len(c.Ai.TransferKeywords) == 0 {
		c.Ai.TransferKeywords = []string{"人工", "转人工", "人工客服"}
	}
	if c.Ai.VectorSimilarityThreshold == 0 {
		c.Ai.VectorSimilarityThreshold = 0.9
	}
	if c.Ai.VectorSearchMinSimilarity == 0 {
		c.Ai.VectorSearchMinSimilarity = 0.75
	}
	if c.Ai.AgentTemperature == 0 {
		c.Ai.AgentTemperature = 0.5
	}
	if c.Ai.TriageTemperature == 0 {
		c.Ai.TriageTemperature = 0.2
	}
	if c.Ai.TriageContextQuestions == 0 {
		c.Ai.TriageContextQuestions = 2
	}
	if c.Ai.TriageMaxHistoryRounds == 0 {
		c.Ai.TriageMaxHistoryRounds = 2
	}
	if c.Ai.TriageTimeout == 0 {
		c.Ai.TriageTimeout = 10
	}
	if c.Ai.TransferGracePeriod == 0 {
		c.Ai.TransferGracePeriod = 5
	}
	if c.Ai.HumanModeGracePeriod == 0 {
		c.Ai.HumanModeGracePeriod = 900
	}
	if c.Ai.ItemCardTTL == 0 {
		c.Ai.ItemCardTTL = 21600
	}
	if c.Ai.AsyncJobTimeout == 0 {
		c.Ai.AsyncJobTimeout = 60
	}
	if c.Ai.ChatMaxHistoryRounds == 0 {
		c.Ai.ChatMaxHistoryRounds = 5
	}
	if c.Ai.MaxReActRounds == 0 {
		c.Ai.MaxReActRounds = 5
	}
	if c.Ai.MaxAssistantPerRound == 0 {
		c.Ai.MaxAssistantPerRound = 5
	}
	if c.Ai.KeywordSyncInterval == 0 {
		c.Ai.KeywordSyncInterval = 300
	}
	if c.Ai.KeywordReloadDebounce == 0 {
		c.Ai.KeywordReloadDebounce = 600
	}
	if c.Ai.HistoryMaxAge == 0 {
		c.Ai.HistoryMaxAge = 86400
	}
	if c.Ai.TransferMsgCooldown == 0 {
		c.Ai.TransferMsgCooldown = 300
	}
	if c.Chatwoot.Teams.PreSalesID == 0 {
		c.Chatwoot.Teams.PreSalesID = 0
	}
	if c.Chatwoot.Teams.AfterSalesID == 0 {
		c.Chatwoot.Teams.AfterSalesID = 0
	}
	if c.Chatwoot.Teams.DefaultID == 0 {
		c.Chatwoot.Teams.DefaultID = 0
	}
	if c.Oss.StoragePath == "" {
		c.Oss.StoragePath = "agent/"
	}
}

// HandleConfigChange 检测配置变化并安全地、并发地重载相关服务
func (i *Initializer) HandleConfigChange(oldConfig, newConfig *config.Config) {
	i.reloadLock.Lock()
	defer i.reloadLock.Unlock()

	var restartNeeded []string

	// --- 1. 检查不可热重载的高风险配置 ---
	if !reflect.DeepEqual(oldConfig.Database, newConfig.Database) {
		restartNeeded = append(restartNeeded, "database")
	}
	if oldConfig.GinAddr != newConfig.GinAddr {
		restartNeeded = append(restartNeeded, "gin_addr")
	}
	if oldConfig.GinLogPath != newConfig.GinLogPath || oldConfig.RunLogPath != newConfig.RunLogPath {
		restartNeeded = append(restartNeeded, "log_path")
	}

	// --- 2. 并发执行可安全热重载的任务 ---
	eg, _ := errgroup.WithContext(context.Background())

	// 时区重载
	if oldConfig.Tz != newConfig.Tz {
		eg.Go(func() error {
			if err := i.InitTz(); err != nil {
				global.Log.Errorf("热重载时区失败: %v", err)
				return err
			}
			return nil
		})
	}

	// Redis客户端重载
	if !reflect.DeepEqual(oldConfig.Redis, newConfig.Redis) {
		eg.Go(func() error {
			if err := i.redisClose(); err != nil {
				global.Log.Warnf("关闭旧Redis客户端失败: %v", err)
			}
			if err := i.initRedis(); err != nil {
				global.Log.Errorf("热重载Redis客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// Chatwoot客户端重载
	if !reflect.DeepEqual(oldConfig.Chatwoot, newConfig.Chatwoot) {
		eg.Go(func() error {
			if err := i.initChatwoot(); err != nil {
				global.Log.Errorf("热重载Chatwoot客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// LLM服务重载
	if !reflect.DeepEqual(oldConfig.Llm, newConfig.Llm) {
		eg.Go(func() error {
			if err := i.initLlm(); err != nil {
				global.Log.Errorf("热重载LLM服务失败: %v", err)
				return err
			}
			return nil
		})
	}

	// 向量化模型服务重载
	if !reflect.DeepEqual(oldConfig.LlmEmbedding, newConfig.LlmEmbedding) {
		eg.Go(func() error {
			if err := i.initLlmEmbedding(); err != nil {
				global.Log.Errorf("热重载向量化模型服务失败: %v", err)
				return err
			}
			return nil
		})
	}

	// 向量数据库客户端重载
	if !reflect.DeepEqual(oldConfig.VectorDb, newConfig.VectorDb) {
		eg.Go(func() error {
			if err := i.vectorDbClose(); err != nil {
				global.Log.Warnf("关闭旧向量数据库客户端失败: %v", err)
			}
			if err := i.initVectorDb(); err != nil {
				global.Log.Errorf("热重载向量数据库客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// AI相关业务逻辑配置重载
	if !reflect.DeepEqual(oldConfig.Ai, newConfig.Ai) {
		eg.Go(func() error {
			// 安全地更新 ActionService 的关键词配置，而不是重建整个 ServiceGroup
			if service.Service.UserServiceGroup.ActionService != nil {
				service.Service.UserServiceGroup.ActionService.UpdateTransferKeywords(newConfig.Ai.TransferKeywords)
				global.Log.Info("热重载AI业务配置(转人工关键词)完成")
			}
			return nil
		})
	}

	// MCP服务重载
	if !reflect.DeepEqual(oldConfig.McpServers, newConfig.McpServers) {
		eg.Go(func() error {
			if global.McpService == nil {
				// 如果之前未初始化，则进行初始化
				if err := i.initMcp(); err != nil {
					global.Log.Errorf("热重载期间初始化MCP服务失败: %v", err)
					return err
				}
				return nil
			}

			oldMap := oldConfig.McpServers
			newMap := newConfig.McpServers

			for name, oldCfg := range oldMap {
				if newCfg, ok := newMap[name]; !ok {
					// 被移除
					global.McpService.RemoveClient(name)
				} else if !reflect.DeepEqual(oldCfg, newCfg) {
					// 被修改
					global.McpService.AddOrUpdateClient(name, newCfg)
				}
			}

			// 新增的
			for name, newCfg := range newMap {
				if _, ok := oldMap[name]; !ok {
					global.McpService.AddOrUpdateClient(name, newCfg)
				}
			}
			return nil
		})
	}

	// OSS 服务重载
	if !reflect.DeepEqual(oldConfig.Oss, newConfig.Oss) {
		eg.Go(func() error {
			if err := i.ossClose(); err != nil {
				global.Log.Warnf("关闭旧OSS客户端失败: %v", err)
			}
			if err := i.initOss(); err != nil {
				global.Log.Errorf("热重载OSS客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		global.Log.Errorf("并发热重载过程中发生错误: %v", err)
	}

	// --- 3. 如果有需要重启的变更，发出统一警告 ---
	if len(restartNeeded) > 0 {
		global.Log.Warnf("检测到存在需要 重启服务 才能生效的配置变更: [%s]。", strings.Join(restartNeeded, ", "))
	}

	global.Log.Info("配置变更处理完成")
}
