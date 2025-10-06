package main

import (
	"fmt"
	"context"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/initialize"
	initGlobal "gitee.com/taoJie_1/chat/initialize/global"
	"gitee.com/taoJie_1/chat/initialize/system"
	"gitee.com/taoJie_1/chat/task"
	"golang.org/x/sync/errgroup"
)

func main() {
	g := initGlobal.New()
	if err := g.InitLog(); err != nil {
		panic(fmt.Sprintf("初始化日志失败[fbvk89]: %v", err))
	}

	defer func() {
		if p := recover(); p != nil {
			global.Log.Errorln(p)
		}
	}()

	eg, _ := errgroup.WithContext(context.Background())

	// 关键任务，失败会终止程序
	eg.Go(g.InitTz)
	eg.Go(system.DbStart)
	eg.Go(initGlobal.InitChatwoot)

	// 非关键任务，失败只打印日志，不影响启动
	eg.Go(func() error {
		initGlobal.InitLlm()
		return nil
	})
	eg.Go(func() error {
		initGlobal.InitLlmEmbedding()
		return nil
	})

	if err := eg.Wait(); err != nil {
		global.Log.Fatalf("关键服务初始化失败，程序终止: %v", err)
	}

	defer system.DbClose()

	initialize.InitializeLogger()

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
