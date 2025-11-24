package task

import (
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
)

var (
	keywordReloadTimer *time.Timer
	keywordReloadMutex sync.Mutex
)

// DebounceKeywordReload 为 KeywordReloader 提供防抖调用功能。
// 每次调用都会重置定时器。
func (m *Manager) DebounceKeywordReload(delay time.Duration) {
	keywordReloadMutex.Lock()
	defer keywordReloadMutex.Unlock()

	// 如果已存在一个定时器，则停止它
	if keywordReloadTimer != nil {
		keywordReloadTimer.Stop()
	}

	// 创建一个新的定时器，在延迟时间后执行同步任务
	keywordReloadTimer = time.AfterFunc(delay, func() {
		global.Log.Info("触发经防抖处理的关键词重同步任务...")
		if err := m.KeywordReloader(); err != nil {
			global.Log.Errorf("执行经防抖处理的关键词重同步任务失败: %v", err)
		}
	})
	global.Log.Infof("关键词重同步任务已调度在 %v 后执行", delay)
}
