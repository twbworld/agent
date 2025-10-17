package main

import (
	"fmt"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/initialize"
	"gitee.com/taoJie_1/mall-agent/task"
)

func main() {
	startTime := time.Now()
	initSvc := initialize.New()
	if err := initSvc.InitLog(); err != nil {
		panic(fmt.Sprintf("初始化日志失败[fbvk89]: %v", err))
	}

	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorln(p)
		}
	}()

	if err := initSvc.Run(); err != nil {
		global.Log.Fatalf("关键服务初始化失败，程序终止: %v", err)
	}
	defer initSvc.Close()

	initialize.InitLogger()

	taskManager := task.NewManager(global.EmbeddingService)

	switch initialize.Act {
	case "":
		initialize.Start(initSvc, taskManager, startTime)
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
