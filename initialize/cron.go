package initialize

import (
	"github.com/robfig/cron/v3"
	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/task"
)

func (i *Initializer) timerStart(taskManager *task.Manager) error {
	i.cron = cron.New([]cron.Option{
		cron.WithLocation(global.Tz),
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
	}...)

	// 每天凌晨1点执行，作为兜底
	if err := i.startCronJob(taskManager.KeywordReloader, "0 1 * * *"); err != nil {
		return err
	}

	if err := i.startCronJob(func() error { return taskManager.McpCapabilitiesReloader() }, "*/35 * * * *"); err != nil {
		return err
	}

	if err := i.startCronJob(taskManager.CleanUpLogs, "0 3 * * *"); err != nil {
		return err
	}

	i.cron.Start() //已含协程
	global.Log.Infoln("定时器启动成功")
	return nil
}

func (i *Initializer) timerStop() {
	if i.cron == nil {
		global.Log.Warnln("定时器未启动")
		return
	}
	i.cron.Stop()
	global.Log.Infoln("定时器停止成功")
}

// 启动一个新的定时任务
func (i *Initializer) startCronJob(task func() error, schedule string) error {
	_, err := i.cron.AddFunc(schedule, func() {
		if err := task(); err != nil {
			global.Log.Errorf("定时任务执行失败 [%s]: %v", schedule, err)
		}
	})
	return err
}
