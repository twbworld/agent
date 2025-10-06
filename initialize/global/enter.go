package global

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/config"
)

type GlobalInit struct {
}

var (
	Conf string
	Act  string
)

func init() {
	flag.StringVar(&Conf, "c", "", "choose config file.")
	flag.StringVar(&Act, "a", "", `行为,默认为空,即启动服务; "clear": 清除过期数据;`)
}

func New(configFile ...string) *GlobalInit {
	var config string
	if gin.Mode() != gin.TestMode {
		//避免 单元测试(go test)自动加参数, 导致flag报错
		flag.Parse() //解析cli命令参数
		if Conf != "" {
			config = Conf
		}
	}
	if config == "" && len(configFile) > 0 {
		config = configFile[0]
	}
	if config == "" {
		config = `config.yaml`
	}

	// 初始化 viper
	v := viper.New()
	v.SetConfigFile(config)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		panic("读取配置失败[u9ij]: " + config + err.Error())
	}

	// 监听配置文件
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("配置文件变化[djiads]: ", e.Name)
		if err := v.Unmarshal(global.Config); err != nil {
			if err := v.Unmarshal(global.Config); err != nil {
				fmt.Println(err)
			}
		}
		handleConfig(global.Config)
	})
	// 将配置赋值给全局变量(结构体需要设置mapstructure的tag)
	if err := v.Unmarshal(global.Config); err != nil {
		panic("出错[dhfal]: " + err.Error())
	}

	handleConfig(global.Config)

	return &GlobalInit{}
}

func InitChatwoot() error {
	if err := initChatwoot(); err != nil {
		global.Log.Warnf("初始化Chatwoot服务失败: %v", err)
		return err
	}
	global.Log.Info("初始化Chatwoot服务成功")
	return nil
}

func InitLlm() {
	if err := initLlm(); err != nil {
		global.Log.Warnf("初始化LLM服务失败: %v", err)
	} else {
		global.Log.Info("初始化LLM服务成功")
	}
}

func InitLlmEmbedding() {
	if err := initLlmEmbedding(); err != nil {
		global.Log.Warnf("初始化向量化服务失败: %v", err)
	} else {
		global.Log.Info("初始化向量化服务成功")
	}
}

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
	if c.Chatwoot.Url == "" {
		c.Chatwoot.Url = "http://127.0.0.1:8080"
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
		c.LlmEmbedding.Timeout = 100
	}
	if c.LlmEmbedding.EmbeddingDim == 0 {
		c.LlmEmbedding.EmbeddingDim = 1024
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
	if len(c.Ai.TransferKeywords) == 0 {
		c.Ai.TransferKeywords = []string{"人工客服", "转人工"}
	}
	if c.Ai.VectorSimilarityThreshold == 0 {
		c.Ai.VectorSimilarityThreshold = 0.9
	}
}
