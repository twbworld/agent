package initialize

import (
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/task"
)

// loadData 加载业务所需数据
func (i *Initializer) loadData(taskManager *task.Manager) {
	if err := taskManager.LoadKeywords(); err != nil {
		global.Log.Errorln("启动时加载Keywords失败, 快捷回复功能将不可用:", err)
	}
}
