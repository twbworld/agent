package system

import "gitee.com/taoJie_1/chat/task"

type systemRes struct{}

// 启动系统资源
func Start(taskManager *task.Manager) *systemRes {
	if err := timerStart(taskManager); err != nil {
		panic(err)
	}
	return &systemRes{}
}

// 关闭系统资源
func (*systemRes) Stop() {
	if err := timerStop(); err != nil {
		panic(err)
	}
}
