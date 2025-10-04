package main

import (
	"fmt"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/initialize"
	initGlobal "gitee.com/taoJie_1/chat/initialize/global"
	"gitee.com/taoJie_1/chat/initialize/system"
	"gitee.com/taoJie_1/chat/task"
)

func main() {
	initGlobal.New().Start()
	initialize.InitializeLogger()
	if err := system.DbStart(); err != nil {
		global.Log.Fatalf("连接数据库失败[fbvk89]: %v", err)
	}
	defer system.DbClose()

	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorln(p)
		}
	}()

	// 直接使用 global.EmbeddingService 创建 taskManager
	taskManager := task.NewManager(global.EmbeddingService)

	switch initGlobal.Act {
	case "":
		initialize.Start(taskManager)
	case "keyword":
		if err := taskManager.KeywordReloader(); err == nil {
			fmt.Println("...执行成功")
		} else {
			fmt.Println("...执行失败: ", err)
		}
	default:
		fmt.Println("参数可选: keyword")
	}

}
