package global

import (
	"sync"
	"time"

	"gitee.com/taoJie_1/chat/internal/chatwoot"
	"gitee.com/taoJie_1/chat/model/config"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// 全局变量
// 业务逻辑禁止修改
var (
	Config          *config.Config = new(config.Config) //指针类型, 给与其内存空间
	Log             *logrus.Logger
	Tz              *time.Location
	Llm             map[enum.LlmSize]*openai.Client = make(map[enum.LlmSize]*openai.Client, 3)
	LlmEmbedding    *openai.Client
	CannedResponses *CannedResponsesMap = &CannedResponsesMap{Data: make(map[string]string)}
	ChatwootClient  *chatwoot.Client
)

type CannedResponsesMap struct {
	sync.RWMutex
	Data map[string]string
}
