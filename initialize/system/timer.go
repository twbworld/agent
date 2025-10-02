package system

import (
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/task"
	"github.com/robfig/cron/v3"
)

var c *cron.Cron

// startCronJob 启动一个新的定时任务
func startCronJob(task func() error, schedule, name string) error {
	_, err := c.AddFunc(schedule, func() {
		if err := task(); err != nil {
			panic(err)
		}
	})
	return err
}

func timerStart(taskManager *task.Manager) error {
	c = cron.New([]cron.Option{
		cron.WithLocation(global.Tz),
		// cron.WithSeconds(), //精确到秒
	}...)

	if err := startCronJob(taskManager.KeywordReloader, "0 3 * * *", "同步关键字"); err != nil {
		return err
	}

	c.Start() //已含协程
	global.Log.Infoln("定时器启动成功")
	return nil
}

func timerStop() error {
	if c == nil {
		global.Log.Warnln("定时器未启动")
		return nil
	}
	c.Stop()
	global.Log.Infoln("定时器停止成功")
	return nil
}
