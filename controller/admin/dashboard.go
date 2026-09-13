package admin

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/twbworld/agent/model/common"
	"github.com/twbworld/agent/service"
)

type DashboardApi struct{}

// GetDashboardDetails 从MCP获取仪表板详情 (用户、商品或订单)，用于在Chatwoot仪表板应用中展示
func (p *DashboardApi) GetDashboardDetails(ctx *gin.Context) {
	var req common.DashboardDetailsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Fail(ctx, errors.New("参数解析失败").Error())
		return
	}

	details, err := service.Service.AdminServiceGroup.DashboardService.GetDetails(ctx, req.UserID, req.GoodsID, req.OrderID)
	if err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	common.Success(ctx, details)
}
