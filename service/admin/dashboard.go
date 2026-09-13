package admin

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/twbworld/agent/global"
	"golang.org/x/sync/errgroup"
)

const (
	// preferredMcpClient 是首选的MCP客户端名称
	preferredMcpClient = "mall-mcp"
	// 工具名称
	mcpToolQueryGoods     = "query_goods"
	mcpToolQueryOrder     = "query_order"
	mcpToolQueryOrderList = "query_order_list"
	mcpToolQueryUser      = "query_user"
	// 参数名称
	mcpArgGoodsId = "goods_id"
	mcpArgOrderId = "order_id"
)

const (
	McpArgUserId = "user_id"
)

type DashboardService interface {
	// GetDetails 调用MCP服务获取用户、商品或订单的聚合详情
	GetDetails(ctx context.Context, userID, goodsID, orderID string) (map[string]any, error)
}

type dashboardService struct{}

func NewDashboardService() DashboardService {
	return &dashboardService{}
}

func (s *dashboardService) getClientName() (string, error) {
	var clientName string
	// 优先使用preferredMcpClient客户端
	if _, ok := global.Config.McpServers[preferredMcpClient]; ok {
		clientName = preferredMcpClient
	} else {
		// 如果找不到，则回退到选择第一个可用的客户端，并发出警告
		for name := range global.Config.McpServers {
			clientName = name // 使用第一个可用的客户端名称
			global.Log.Warnf("未找到首选的MCP客户端 '%s'，已回退到使用第一个可用的客户端 '%s'", preferredMcpClient, clientName)
			break
		}
	}

	if clientName == "" {
		return "", errors.New("未配置任何MCP服务客户端")
	}
	return clientName, nil
}

func (s *dashboardService) getGoodsDetails(ctx context.Context, clientName, goodsID string) (map[string]any, error) {
	argsMap := map[string]any{
		mcpArgGoodsId: goodsID,
	}
	arguments, _ := json.Marshal(argsMap)

	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, mcpToolQueryGoods, jsontext.Value(arguments))
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s.%s 失败: %w", clientName, mcpToolQueryGoods, err)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的商品详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) getOrderDetails(ctx context.Context, clientName, orderID string) (map[string]any, error) {
	argsMap := map[string]any{
		mcpArgOrderId: orderID,
	}
	arguments, _ := json.Marshal(argsMap)

	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, mcpToolQueryOrder, jsontext.Value(arguments))
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s.%s 失败: %w", clientName, mcpToolQueryOrder, err)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的订单详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) getOrderList(ctx context.Context, clientName, userID string) (map[string]any, error) {
	argsMap := map[string]any{
		McpArgUserId: userID,
		"page_no":    1,
		"page_size":  10,
	}
	arguments, _ := json.Marshal(argsMap)

	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, mcpToolQueryOrderList, jsontext.Value(arguments))
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s.%s 失败: %w", clientName, mcpToolQueryOrderList, err)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的订单详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) getUserDetails(ctx context.Context, clientName, userID string) (map[string]any, error) {
	argsMap := map[string]any{
		McpArgUserId: userID,
	}
	arguments, _ := json.Marshal(argsMap)

	resultStr, err := global.McpService.ExecuteTool(ctx, clientName, mcpToolQueryUser, jsontext.Value(arguments))
	if err != nil {
		return nil, fmt.Errorf("调用MCP工具 %s.%s 失败: %w", clientName, mcpToolQueryUser, err)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(resultStr), &details); err != nil {
		return nil, fmt.Errorf("解析MCP返回的用户详情JSON失败: %w, 原始返回: %s", err, resultStr)
	}
	return details, nil
}

func (s *dashboardService) GetDetails(ctx context.Context, userID, goodsID, orderID string) (map[string]any, error) {
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
	data := make(map[string]any)
	var firstErr error
	g, gCtx := errgroup.WithContext(ctx)

	if userID != "" {
		g.Go(func() error {
			details, err := s.getUserDetails(gCtx, clientName, userID)
			if err != nil {
				global.Log.Errorf("%v", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return nil
			}
			mu.Lock()
			data["user"] = details
			mu.Unlock()
			return nil
		})
		g.Go(func() error {
			details, err := s.getOrderList(gCtx, clientName, userID)
			if err != nil {
				global.Log.Errorf("%v", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return nil
			}
			mu.Lock()
			data["order_list"] = details
			mu.Unlock()
			return nil
		})
	}

	if goodsID != "" {
		g.Go(func() error {
			details, err := s.getGoodsDetails(gCtx, clientName, goodsID)
			if err != nil {
				if strings.Contains(err.Error(), "上架") || strings.Contains(err.Error(), "下架") {
					// 商品可能已下架，商城api会响应mcp错误, 这里模拟一个下架的商品详情返回
					details = map[string]any{
						"status": "已下架",
						"id":     goodsID,
					}
				} else {
					global.Log.Errorf("%v", err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return nil
				}
			}
			mu.Lock()
			data["product"] = details
			mu.Unlock()
			return nil
		})
	}

	if orderID != "" {
		g.Go(func() error {
			details, err := s.getOrderDetails(gCtx, clientName, orderID)
			if err != nil {
				global.Log.Errorf("%v", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return nil
			}
			mu.Lock()
			data["order"] = details
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(data) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("未能获取到任何详情信息")
	}

	return data, nil
}
