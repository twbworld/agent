package initialize

import (
	"flag"
	"fmt"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/config"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
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
func New() *Initializer {
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
	i := &Initializer{}

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
	const debounceDuration = 200 * time.Millisecond

	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		debounceMutex.Lock()
		// 如果已有计时器，则重置它
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		// 启动一个新的计时器，在持续时间内如果没有新事件，则执行重载
		debounceTimer = time.AfterFunc(debounceDuration, func() {
			fmt.Println("配置文件变化[djiads] (Debounced): ", e.Name)

			// 为比较，先深度拷贝一份旧的配置
			oldConfig := global.Config.DeepCopy()

			if err := v.Unmarshal(global.Config); err != nil {
				fmt.Println("热加载配置文件反序列化失败:", err)
				// 如果反序列化失败，则恢复旧配置，防止程序状态不一致
				global.Config = oldConfig
				return
			}
			handleConfig(global.Config)

			// 调用新的处理函数来处理配置变更
			i.HandleConfigChange(oldConfig, global.Config)
		})
		debounceMutex.Unlock()
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
		c.ProjectName = "AI客服系统"
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
			c.Llm[i].Timeout = 10
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
		c.Ai.TransferKeywords = []string{"人工客服", "转人工"}
	}
	if c.Ai.VectorSimilarityThreshold == 0 {
		c.Ai.VectorSimilarityThreshold = 0.9
	}
	if c.Ai.TriageContextQuestions == 0 {
		c.Ai.TriageContextQuestions = 2
	}
	if c.Ai.AsyncJobTimeout == 0 {
		c.Ai.AsyncJobTimeout = 30
	}
	if c.Ai.TransferGracePeriod == 0 {
		c.Ai.TransferGracePeriod = 5
	}
}
