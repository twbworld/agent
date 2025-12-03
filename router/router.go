package router

import (
	"net/http"
	"strings"

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

	ginServer.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "index.html", nil)
	})

	ginServer.NoRoute(func(ctx *gin.Context) {
		if strings.Contains(ctx.Request.Header.Get("Accept"), "text/html") {
			ctx.HTML(http.StatusNotFound, "404.html", gin.H{"status": "404"})
		} else {
			common.FailNotFound(ctx)
		}
	})

	v1 := ginServer.Group("api/v1")
	{
		v1.POST("/chat", controller.Api.UserApiGroup.ChatApi.HandleWebhook)
		v1.POST("/mcp/reload", controller.Api.UserApiGroup.BaseApi.Reload)
		v1.POST("/chatwoot/details", controller.Api.UserApiGroup.DashboardApi.GetDashboardDetails)

		// 知识库管理页面的 API 路由
		adminRoutes := v1.Group("/admin")
		{
			keywordRoutes := adminRoutes.Group("/keywords")
			{
				keywordRoutes.GET("", controller.Api.AdminApiGroup.KeywordApi.ListItems)
				keywordRoutes.POST("", controller.Api.AdminApiGroup.KeywordApi.UpsertItem)
				keywordRoutes.DELETE("/:id", controller.Api.AdminApiGroup.KeywordApi.DeleteItem)
				keywordRoutes.POST("/generate-questions", controller.Api.AdminApiGroup.KeywordApi.GenerateQuestions)
				keywordRoutes.POST("/force-sync", controller.Api.AdminApiGroup.KeywordApi.ForceSync)
			}
			adminRoutes.POST("/upload/image", controller.Api.AdminApiGroup.UploadApi.UploadImage)
		}
	}

	chatwootGroup := ginServer.Group("/chatwoot")
	{
		chatwootGroup.GET("/dashboard", func(ctx *gin.Context) {
			ctx.HTML(http.StatusOK, "contact_details.html", nil)
		})
	}

	// 知识库管理 HTML 页面路由
	ginServer.GET("/keyword", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "keyword.html", nil)
	})

}
