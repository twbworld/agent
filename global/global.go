package global

import (
	"context"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/embedding"
	"gitee.com/taoJie_1/mall-agent/internal/llm"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/internal/vector"
	"gitee.com/taoJie_1/mall-agent/model/config"
	"github.com/sirupsen/logrus"
)

// 全局变量
// 业务逻辑禁止修改
var (
	Config           *config.Config = new(config.Config) //指针类型, 给与其内存空间
	Log              *logrus.Logger
	Tz               *time.Location
	CannedResponses  *CannedResponsesMap = &CannedResponsesMap{Data: make(map[string]string)}
	ChatwootService  chatwoot.Service
	EmbeddingService embedding.Service
	LlmService       llm.Service
	VectorDb         vector.Service
	RedisClient      redis.Service
	ActiveLLMTasks *ActiveTasksMap = &ActiveTasksMap{Data: make(map[uint]context.CancelFunc)}
)

type CannedResponsesMap struct {
	sync.RWMutex
	Data map[string]string
}

// ActiveTasksMap 用于存储正在进行的异步任务的取消函数
type ActiveTasksMap struct {
	sync.RWMutex
	Data map[uint]context.CancelFunc
}
