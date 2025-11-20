package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gitee.com/taoJie_1/mall-agent/global"
)

const (
	// MCPP_TOOL_GET_GOODS_DETAILS 是从MCP获取商品详情的工具名称
	MCPP_TOOL_GET_GOODS_DETAILS = "query_goods"
	// MCP_ARG_GOODS_ID 是MCP获取商品详情工具的商品ID参数名
	MCP_ARG_GOODS_ID = "goods_id"

	// MCP_TOOL_GET_ORDER_DETAILS 是从MCP获取订单详情的工具名称
	MCP_TOOL_GET_ORDER_DETAILS = "query_order"
	// MCP_ARG_ORDER_ID 是MCP获取订单详情工具的订单ID参数名
	MCP_ARG_ORDER_ID = "order_id"

	// PreferredMcpClient 是首选的MCP客户端名称
	PreferredMcpClient = "mall-mcp"
)

type DashboardService interface {
	// GetDetails 调用MCP服务获取商品或订单详情
	GetDetails(ctx context.Context, goodsID, orderID string) (map[string]interface{}, error)
}

type dashboardService struct{}

func NewDashboardService() DashboardService {
	return &dashboardService{}
}

func (s *dashboardService) GetDetails(ctx context.Context, goodsID, orderID string) (map[string]interface{}, error) {
	if global.McpService == nil {
		return nil, errors.New("MCP服务未初始化")
	}

	if goodsID == "" && orderID == "" {
		return nil, errors.New("商品ID和订单ID不能同时为空")
	}

	var clientName string
	// 优先使用PreferredMcpClient客户端
	if _, ok := global.Config.McpServers[PreferredMcpClient]; ok {
		clientName = PreferredMcpClient
	} else {
		// 如果找不到，则回退到选择第一个可用的客户端，并发出警告
		for name := range global.Config.McpServers {
			clientName = name // 使用第一个可用的客户端名称
			global.Log.Warnf("未找到首选的MCP客户端 '%s'，已回退到使用第一个可用的客户端 '%s'", PreferredMcpClient, clientName)
			break
		}
	}

	if clientName == "" {
		return nil, errors.New("未配置任何MCP服务客户端")
	}

	var resultStr string
	var err error

	if goodsID != "" {
		// 构造工具调用参数
		arguments := json.RawMessage(fmt.Sprintf(`{"%s": "%s"}`, MCP_ARG_GOODS_ID, goodsID))
		// 执行工具调用
		resultStr, err = global.McpService.ExecuteTool(ctx, clientName, MCPP_TOOL_GET_GOODS_DETAILS, arguments)
		if err != nil {
			return nil, fmt.Errorf("调用MCP工具 %s 失败: %w", MCPP_TOOL_GET_GOODS_DETAILS, err)
		}
	} else if orderID != "" {
		// 构造工具调用参数
		arguments := json.RawMessage(fmt.Sprintf(`{"%s": "%s"}`, MCP_ARG_ORDER_ID, orderID))
		// 执行工具调用
		resultStr, err = global.McpService.ExecuteTool(ctx, clientName, MCP_TOOL_GET_ORDER_DETAILS, arguments)
		if err != nil {
			return nil, fmt.Errorf("调用MCP工具 %s 失败: %w", MCP_TOOL_GET_ORDER_DETAILS, err)
		}
	}

	// 解析返回的JSON字符串
	var details map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}

	return details, nil
}
