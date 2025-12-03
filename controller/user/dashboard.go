package user

import (
	"errors"

	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/service"
	"github.com/gin-gonic/gin"
)

type DashboardApi struct{}

// GetDashboardDetails 从MCP获取仪表板详情 (用户、商品或订单)，用于在Chatwoot仪表板应用中展示
func (p *DashboardApi) GetDashboardDetails(ctx *gin.Context) {
	var req common.DashboardDetailsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Fail(ctx, errors.New("参数解析失败").Error())
		return
	}

	details, err := service.Service.UserServiceGroup.DashboardService.GetDetails(ctx, req.UserID, req.GoodsID, req.OrderID)
	if err != nil {
		common.Fail(ctx, err.Error())
		return
	}

	common.Success(ctx, details)
}
