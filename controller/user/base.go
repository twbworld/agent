package user

import (
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/task"
	"github.com/gin-gonic/gin"
)


type BaseApi struct{}

//MCP能力刷新webhook
func (m *BaseApi) Reload(ctx *gin.Context) {
	taskManager := task.NewManager(nil)

	go func() {
		if err := taskManager.McpCapabilitiesReloader(); err != nil {
			global.Log.Errorf("通过API触发MCP能力刷新失败: %v", err)
		}
	}()

	common.Success(ctx, "MCP能力刷新任务已启动")
}
