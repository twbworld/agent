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

		// 知识库管理页面的 API 路由
		adminRoutes := v1.Group("/admin")
		// 为所有 admin 接口添加鉴权中间件
		adminRoutes.Use(middleware.AuthCheck)
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

			adminRoutes.POST("/dashboard/details", controller.Api.AdminApiGroup.DashboardApi.GetDashboardDetails)
		}
	}

	// 知识库管理 HTML 页面路由, 增加页面鉴权
	ginServer.GET("/keyword", middleware.PageAuthCheck, func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "keyword.html", nil)
	})

	// Chatwoot仪表板应用 HTML 页面路由; 在chatwoot下"集成方式-仪表板应用"配置的, 用于在客服会话界面展示的页面, 增加页面鉴权
	ginServer.GET("/admin/dashboard", middleware.PageAuthCheck, func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "contact_details.html", nil)
	})

}
