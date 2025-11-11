package task

import (
	"fmt"
	"strings"
	"sync"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/config"
)

// McpCapabilitiesReloader 刷新所有或指定的 MCP 服务的能力。
// 如果没有提供 mcpName，它会遍历配置中定义的所有 MCP 服务器。
// 如果提供了 mcpName，它只会刷新指定的 MCP 服务。
// 这确保了代理对 MCP 能力的了解是最新的。
func (m *Manager) McpCapabilitiesReloader(mcpName ...string) error {
	if global.McpService == nil {
		global.Log.Info("MCP服务未启用，跳过能力刷新任务")
		return nil
	}

	mcpConfigs := global.Config.McpServers
	if len(mcpConfigs) == 0 {
		global.Log.Info("未配置任何MCP服务，跳过能力刷新任务")
		return nil
	}

	// 如果提供了mcpName且不为空，则只刷新指定的MCP
	if len(mcpName) > 0 && mcpName[0] != "" {
		targetMcpName := mcpName[0]
		cfg, ok := mcpConfigs[targetMcpName]
		if !ok {
			err := fmt.Errorf("未在配置中找到名为 '%s' 的MCP服务,刷新操作已中止", targetMcpName)
			global.Log.Warn(err)
			return err
		}

		global.Log.Infof("开始刷新MCP服务 '%s' 的能力...", targetMcpName)
		if err := global.McpService.AddOrUpdateClient(targetMcpName, cfg); err != nil {
			finalError := fmt.Errorf("刷新MCP '%s' 能力时发生错误: %w", targetMcpName, err)
			global.Log.Warn(finalError)
			return finalError
		}
		global.Log.Infof("MCP服务 '%s' 的能力刷新完成", targetMcpName)
		return nil
	}

	// 否则，刷新所有MCP服务
	global.Log.Info("开始刷新所有MCP服务能力...")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for name, cfg := range mcpConfigs {
		wg.Add(1)
		go func(name string, cfg config.Mcp) {
			defer wg.Done()
			// AddOrUpdateClient 是线程安全的，它处理连接、发现工具和更新内部缓存的逻辑。
			err := global.McpService.AddOrUpdateClient(name, cfg)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("刷新MCP客户端 '%s' 失败: %w", name, err))
				mu.Unlock()
			}
		}(name, cfg)
	}

	wg.Wait()

	if len(errs) > 0 {
		var combinedErr strings.Builder
		for _, e := range errs {
			combinedErr.WriteString(e.Error())
			combinedErr.WriteString("; ")
		}
		finalError := fmt.Errorf("刷新MCP能力时发生错误: %s", strings.TrimRight(combinedErr.String(), "; "))
		global.Log.Error(finalError)
		return finalError
	}

	global.Log.Info("所有MCP服务能力刷新完成")
	return nil
}
