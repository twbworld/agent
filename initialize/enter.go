package initialize

import (
	"context"
	"fmt"
	"io"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/task"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

// Initializer 统一管理项目的所有初始化工作
type Initializer struct {
	cron          *cron.Cron
	logFileCloser io.Closer // 用于存储日志文件的关闭器
}

// Run 并发执行所有核心服务的初始化
func (i *Initializer) Run() error {
	eg, _ := errgroup.WithContext(context.Background())

	// 关键任务，失败会终止程序
	eg.Go(i.initTz)
	eg.Go(i.dbStart)
	eg.Go(i.initChatwoot)
	eg.Go(i.initRedis)

	// 非关键任务，失败只打印日志，不影响启动
	eg.Go(func() error {
		_ = i.initVectorDb()
		return nil
	})
	eg.Go(func() error {
		_ = i.initLlm()
		return nil
	})
	eg.Go(func() error {
		_ = i.initLlmEmbedding()
		return nil
	})

	return eg.Wait()
}

// Close 优雅地关闭和释放所有资源
func (i *Initializer) Close() {
	if i.vectorDbClose() == nil {
		global.Log.Info("VectorDb客户端已关闭")
	}
	if i.dbClose() == nil {
		global.Log.Infof("%s已关闭", global.Config.Database.Type)
	}
	i.timerStop()
	_ = i.logClose()
}

// logClose 关闭或刷新日志组件
func (i *Initializer) logClose() error {
	if i.logFileCloser != nil {
		return i.logFileCloser.Close()
	}
	return nil
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

// initRedis 初始化Redis客户端
func (i *Initializer) initRedis() error {
	client, err := redis.NewClient(
		global.Config.Redis.Addr,
		global.Config.Redis.Password,
		global.Config.Redis.DB,
	)
	if err != nil {
		return fmt.Errorf("初始化Redis客户端失败: %w", err)
	}
	global.RedisClient = client
	global.Log.Info("初始化Redis服务成功")
	return nil
}
