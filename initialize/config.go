package initialize

import (
	"flag"
	"fmt"
	"strings"

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

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		panic("读取配置失败[u9ij]: " + configPath + err.Error())
	}

	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("配置文件变化[djiads]: ", e.Name)
		if err := v.Unmarshal(global.Config); err != nil {
			fmt.Println(err)
		}
		handleConfig(global.Config)
	})

	if err := v.Unmarshal(global.Config); err != nil {
		panic("出错[dhfal]: " + err.Error())
	}

	handleConfig(global.Config)

	return &Initializer{}
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
	if c.Redis.LockPrefix == "" {
		c.Redis.LockPrefix = "agent:lock:"
	}
	if c.Redis.LockExpiry == 0 {
		c.Redis.LockExpiry = 30
	}
	if c.Redis.ConversationHistoryTTL == 0 { // 新增
		c.Redis.ConversationHistoryTTL = 3600 // 默认1小时
	}
	if c.Chatwoot.Url == "" {
		c.Chatwoot.Url = "http://127.0.0.1:8080"
	}
	if c.Chatwoot.AccountId == 0 {
		c.Chatwoot.AccountId = 1
	}
	if c.Chatwoot.AgentUserID == 0 {
		c.Chatwoot.AgentUserID = 2
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
	if c.Ai.AsyncJobTimeout == 0 {
		c.Ai.AsyncJobTimeout = 30
	}
}
