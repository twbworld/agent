package initialize

import (
	"context"
	"fmt"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/task"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

// Initializer 统一管理项目的所有初始化工作
type Initializer struct {
	cron *cron.Cron
}

// Run 并发执行所有核心服务的初始化
func (i *Initializer) Run() error {
	eg, _ := errgroup.WithContext(context.Background())

	// 关键任务，失败会终止程序
	eg.Go(i.initTz)
	eg.Go(i.dbStart)
	eg.Go(i.initChatwoot)

	// 非关键任务，失败只打印日志，不影响启动
	eg.Go(func() error {
		i.initLlm()
		return nil
	})
	eg.Go(func() error {
		i.initLlmEmbedding()
		return nil
	})

	return eg.Wait()
}

// Close 优雅地关闭和释放所有资源
func (i *Initializer) Close() {
	i.dbClose()
	i.timerStop()
}

// StartSystem 启动系统级服务，如定时器和数据加载
func (i *Initializer) StartSystem(taskManager *task.Manager) {
	if err := i.timerStart(taskManager); err != nil {
		panic(err)
	}
	i.loadData(taskManager)
}

func (i *Initializer) initTz() error {
	Location, err := time.LoadLocation(global.Config.Tz)
	if err != nil {
		return fmt.Errorf("时区配置失败[siortuj]: %w", err)
	}
	global.Tz = Location
	return nil
}