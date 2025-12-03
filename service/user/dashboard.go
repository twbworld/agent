package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"gitee.com/taoJie_1/mall-agent/global"
	"golang.org/x/sync/errgroup"
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

	// MCP_TOOL_GET_USER_DETAILS 是从MCP获取用户详情的工具名称
	MCP_TOOL_GET_USER_DETAILS = "query_user"
	// MCP_ARG_USER_ID 是MCP获取用户详情工具的用户ID参数名
	MCP_ARG_USER_ID = "user_id"

	// PreferredMcpClient 是首选的MCP客户端名称
	PreferredMcpClient = "mall-mcp"
)

type DashboardService interface {
	// GetDetails 调用MCP服务获取用户、商品或订单的聚合详情
	GetDetails(ctx context.Context, userID, goodsID, orderID string) (map[string]interface{}, error)
}

type dashboardService struct{}

func NewDashboardService() DashboardService {
	return &dashboardService{}
}

func (s *dashboardService) getClientName() (string, error) {
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
		return "", errors.New("未配置任何MCP服务客户端")
	}
	return clientName, nil
}

func (s *dashboardService) _getGoodsDetails(ctx context.Context, clientName, goodsID string) (map[string]interface{}, error) {
	arguments := json.RawMessage(fmt.Sprintf(`{"%s": "%s"}`, MCP_ARG_GOODS_ID, goodsID))
	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, MCPP_TOOL_GET_GOODS_DETAILS, arguments)
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s 失败: %w", MCPP_TOOL_GET_GOODS_DETAILS, err)
	}

	var details map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的商品详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) _getOrderDetails(ctx context.Context, clientName, orderID string) (map[string]interface{}, error) {
	arguments := json.RawMessage(fmt.Sprintf(`{"%s": "%s"}`, MCP_ARG_ORDER_ID, orderID))
	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, MCP_TOOL_GET_ORDER_DETAILS, arguments)
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s 失败: %w", MCP_TOOL_GET_ORDER_DETAILS, err)
	}

	var details map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的订单详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) _getUserDetails(ctx context.Context, clientName, userID string) (map[string]interface{}, error) {
	arguments := json.RawMessage(fmt.Sprintf(`{"%s": "%s"}`, MCP_ARG_USER_ID, userID))
	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, MCP_TOOL_GET_USER_DETAILS, arguments)
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s 失败: %w", MCP_TOOL_GET_USER_DETAILS, err)
	}

	var details map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的用户详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) GetDetails(ctx context.Context, userID, goodsID, orderID string) (map[string]interface{}, error) {
	if global.McpService == nil {
		return nil, errors.New("MCP服务未初始化")
	}

	if userID == "" && goodsID == "" && orderID == "" {
		return nil, errors.New("用户ID、商品ID和订单ID不能同时为空")
	}

	clientName, err := s.getClientName()
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	allDetails := make(map[string]interface{})
	g, gCtx := errgroup.WithContext(ctx)

	if userID != "" {
		g.Go(func() error {
			details, err := s._getUserDetails(gCtx, clientName, userID)
			if err != nil {
				global.Log.Errorf("获取用户详情失败: %v", err)
				return nil
			}
			mu.Lock()
			allDetails["user"] = details
			mu.Unlock()
			return nil
		})
	}

	if goodsID != "" {
		g.Go(func() error {
			details, err := s._getGoodsDetails(gCtx, clientName, goodsID)
			if err != nil {
				global.Log.Errorf("获取商品详情失败: %v", err)
				return nil
			}
			mu.Lock()
			allDetails["product"] = details
			mu.Unlock()
			return nil
		})
	}

	if orderID != "" {
		g.Go(func() error {
			details, err := s._getOrderDetails(gCtx, clientName, orderID)
			if err != nil {
				global.Log.Errorf("获取订单详情失败: %v", err)
				return nil
			}
			mu.Lock()
			allDetails["order"] = details
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(allDetails) == 0 {
		return nil, errors.New("未能获取到任何详情信息")
	}

	return allDetails, nil
}
