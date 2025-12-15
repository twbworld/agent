package middleware

import (
	"net/http"
	"strings"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// 跨域
func CorsHandle() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowOrigins = global.Config.Cors
	config.AllowMethods = []string{"OPTIONS", "POST", "GET"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "authorization"}
	return cors.New(config)
}

func OptionsMethod(ctx *gin.Context) {
	if ctx.Request.Method == "OPTIONS" {
		ctx.AbortWithStatus(http.StatusNoContent)
	}
}

// AuthCheck 简单的令牌鉴权中间件
func AuthCheck(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		token = c.Query("token")
	}

	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" || (global.Config.Auth.Dashboard != "" && token != global.Config.Auth.Dashboard) {
		common.FailAuth(c, "认证失败: 无效的令牌")
		c.Abort()
		return
	}

	c.Next()
}
