package initialize

import (
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/task"
	"github.com/robfig/cron/v3"
)

func (i *Initializer) timerStart(taskManager *task.Manager) error {
	i.cron = cron.New([]cron.Option{
		cron.WithLocation(global.Tz),
	}...)
	if err := i.startCronJob(taskManager.KeywordReloader, "*/30 * * * *"); err != nil {
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
			panic(err)
		}
	})
	return err
}
