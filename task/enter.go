package task

import (
	"gitee.com/taoJie_1/chat/pkg/embedding"
)

type Manager struct {
	embeddingService embedding.Service
}

// NewManager 创建一个新的任务管理器
func NewManager(embeddingService embedding.Service) *Manager {
	return &Manager{
		embeddingService: embeddingService,
	}
}
