package global

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twbworld/agent/internal/chatwoot"
	"github.com/twbworld/agent/internal/embedding"
	"github.com/twbworld/agent/internal/llm"
	"github.com/twbworld/agent/internal/mcp"
	"github.com/twbworld/agent/internal/oss"
	"github.com/twbworld/agent/internal/redis"
	"github.com/twbworld/agent/internal/vector"
	"github.com/twbworld/agent/model/config"
)

// 全局变量
// 业务逻辑禁止修改
var (
	Version          string
	ConfigLock       sync.RWMutex
	Config           *config.Config = new(config.Config) //指针类型, 给与其内存空间
	Log              *logrus.Logger
	Tz               *time.Location
	RedisClient      redis.Service
	CannedResponses  *CannedResponsesMap = &CannedResponsesMap{Data: make(map[string]string)}
	ChatwootService  chatwoot.Service
	EmbeddingService embedding.Service
	LlmService       llm.Service
	VectorDb         vector.Service
	McpService       mcp.Service
	OssService       oss.Service
	ActiveLLMTasks   *ActiveTasksMap = &ActiveTasksMap{Data: make(map[uint]TaskInfo)}
)

type CannedResponsesMap struct {
	sync.RWMutex
	Data map[string]string
}

// TaskInfo 封装异步任务的控制信息
type TaskInfo struct {
	Cancel    context.CancelFunc
	MessageID uint
}

// ActiveTasksMap 用于存储正在进行的异步任务
type ActiveTasksMap struct {
	sync.RWMutex
	Data map[uint]TaskInfo
}
