package task

import (
	"gitee.com/taoJie_1/chat/internal/embedding"
)

type Manager struct {
	embeddingService embedding.Service
}

// 任务管理器
func NewManager(embeddingService embedding.Service) *Manager {
	return &Manager{
		embeddingService: embeddingService,
	}
}
