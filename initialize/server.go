package initialize

import (
	"context"
	"gitee.com/taoJie_1/mall-agent/task"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/router"
	"gitee.com/taoJie_1/mall-agent/service"
	"gitee.com/taoJie_1/mall-agent/service/admin"
	"gitee.com/taoJie_1/mall-agent/service/common"
	"gitee.com/taoJie_1/mall-agent/service/user"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/gin-gonic/gin"
)

var server *http.Server

func (i *Initializer) InitLogger() {
	ginfile, err := i.setupLogFile(global.Config.GinLogPath)
	if err != nil {
		// 在此处使用 Fatalf 是合适的，因为如果Gin日志无法初始化，服务继续运行可能会隐藏问题。
		global.Log.Fatalf("初始化Gin日志失败: %v", err)
	}

	// 将Gin日志同时输出到文件和标准输出，便于调试
	gin.DefaultWriter = io.MultiWriter(os.Stdout, ginfile)
	gin.DefaultErrorWriter = gin.DefaultWriter
	gin.DisableConsoleColor() //将日志写入文件时不需要控制台颜色
}

func Start(initializer *Initializer, taskManager *task.Manager, startTime time.Time) {
	initializer.StartSystem(taskManager)

	service.Service.CommonServiceGroup = common.NewServiceGroup()
	service.Service.UserServiceGroup = user.NewServiceGroup()
	service.Service.AdminServiceGroup = admin.NewServiceGroup()

	initGinServer()
	//协程启动服务
	go startServer()

	logStartupInfo(startTime)

	waitForShutdown()
}

func initGinServer() {
	mode := gin.ReleaseMode
	if global.Config.Debug {
		mode = gin.DebugMode
	}
	gin.SetMode(mode)

	ginServer := gin.New()
	// 使用 gin.Logger() 和 gin.Recovery() 中间件来替代 gin.Default()
	// 这可以消除 gin.Default() 在调试模式下产生的警告，同时保持功能不变
	ginServer.Use(gin.Logger(), gin.Recovery())
	router.Start(ginServer)

	ginServer.ForwardedByClientIP = true

	// ginServer.Run(":80")
	server = &http.Server{
		Addr:    global.Config.GinAddr,
		Handler: ginServer,
	}
}

// 启动HTTP服务器
func startServer() {
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		global.Log.Panic("服务出错[isjfio]: ", err.Error()) //外部并不能捕获Panic
	}
}

// 记录启动信息
func logStartupInfo(startTime time.Time) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	global.Log.Infof("服务已启动, 耗时: %v, Go: %s, 端口: %s, 模式: %s, PID: %d, 内存: %gMiB", time.Since(startTime), runtime.Version(), global.Config.GinAddr, gin.Mode(), syscall.Getpid(), utils.NumberFormat(float32(m.Alloc)/1024/1024))

}

// 等待关闭信号(ctrl+C)
func waitForShutdown() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done() //阻塞等待

	//来到这 证明有关闭指令,将进行平滑优雅关闭服务

	global.Log.Infof("程序关闭中..., port: %s, pid: %d", global.Config.GinAddr, syscall.Getpid())

	shutdownServer()
}

// 平滑关闭服务器
func shutdownServer() {
	//给程序最多5秒处理余下请求
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//关闭监听端口
	if err := server.Shutdown(timeoutCtx); err != nil {
		global.Log.Panicln("服务关闭出错[oijojiud]", err)
	}
	global.Log.Infoln("服务退出成功")
}
