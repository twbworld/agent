package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/model/common"
)

// AuthCheck 简单的令牌鉴权中间件(API通用)
func AuthCheck(c *gin.Context) {
	if !validateToken(c) {
		common.FailAuth(c, "认证失败: 无效的令牌")
		c.Abort()
		return
	}
	c.Next()
}

// PageAuthCheck 页面鉴权中间件(HTML专用)
func PageAuthCheck(c *gin.Context) {
	if !validateToken(c) {
		c.HTML(http.StatusUnauthorized, "40x.html", gin.H{"status": "401"})
		c.Abort()
		return
	}
	c.Next()
}

// validateToken 统一验证逻辑
func validateToken(c *gin.Context) bool {
	token := c.GetHeader("Authorization")
	if token == "" {
		token = c.Query("token")
	}

	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		return false
	}

	// 验证 Chatwoot Dashboard Token
	if global.Config.Auth.Dashboard != "" && token == global.Config.Auth.Dashboard {
		return true
	}

	// 验证 Mall Admin Token
	if global.Config.Auth.Keyword != "" && token == global.Config.Auth.Keyword {
		return true
	}

	return false
}
