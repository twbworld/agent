package router

import (
	"net/http"

	"gitee.com/taoJie_1/mall-agent/controller"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/middleware"
	"gitee.com/taoJie_1/mall-agent/model/common"

	"github.com/gin-gonic/gin"
)

func Start(ginServer *gin.Engine) {
	// 限制form内存(默认32MiB)
	ginServer.MaxMultipartMemory = 32 << 20

	ginServer.Use(middleware.CorsHandle(), middleware.OptionsMethod) //全局中间件

	ginServer.StaticFile("/favicon.ico", global.Config.StaticDir+"/favicon.ico")
	ginServer.StaticFile("/robots.txt", global.Config.StaticDir+"/robots.txt")
	ginServer.LoadHTMLGlob(global.Config.StaticDir + "/*.html")
	ginServer.StaticFS("/static", http.Dir(global.Config.StaticDir))

	// 错误处理路由
	errorRoutes := []string{"404.html", "40x.html", "50x.html"}
	for _, route := range errorRoutes {
		ginServer.GET(route, func(ctx *gin.Context) {
			ctx.HTML(http.StatusOK, "404.html", gin.H{"status": route[:3]})
		})
		ginServer.POST(route, func(ctx *gin.Context) {
			common.FailNotFound(ctx)
		})
	}

	ginServer.NoRoute(func(ctx *gin.Context) {
		//内部重定向
		ctx.Request.URL.Path = "/404.html"
		ginServer.HandleContext(ctx)
		//http重定向
		// ctx.Redirect(http.StatusMovedPermanently, "/404.html")
	})

	v1 := ginServer.Group("api/v1")
	{
		v1.POST("/chat", controller.Api.UserApiGroup.ChatApi.HandleChat)
	}

}
