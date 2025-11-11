package user

import (
	"fmt"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/task"
	"github.com/gin-gonic/gin"
)

type BaseApi struct{}

// MCP能力刷新webhook
func (m *BaseApi) Reload(ctx *gin.Context) {
	var data common.ReloadPost
	if ctx.ShouldBindJSON(&data) != nil {
		common.Fail(ctx, `参数错误`)
		return
	}

	if data.Name == "" {
		common.Fail(ctx, "非法请求")
		return
	}

	go func() {
		taskManager := task.NewManager(nil)
		if err := taskManager.McpCapabilitiesReloader(data.Name); err != nil {
			global.Log.Warnf("通过API触发MCP能力刷新任务执行时发生错误: %v", err)
		}
	}()

	common.Success(ctx, fmt.Sprintf("MCP服务 '%s' 的能力刷新任务已启动", data.Name))
}
