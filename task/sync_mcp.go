package task

import (
	"fmt"
	"strings"
	"sync"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/config"
)

// McpCapabilitiesReloader 刷新所有已配置的 MCP 服务的能力。
// 它遍历配置中定义的所有 MCP 服务器，并为每个服务器重新建立连接，
// 以发现并缓存最新的可用工具。这确保了代理对 MCP 能力的了解是最新的。
func (m *Manager) McpCapabilitiesReloader() error {
	if global.McpService == nil {
		global.Log.Info("MCP服务未启用，跳过能力刷新任务")
		return nil
	}

	mcpConfigs := global.Config.McpServers
	if len(mcpConfigs) == 0 {
		global.Log.Info("未配置任何MCP服务，跳过能力刷新任务")
		return nil
	}

	global.Log.Info("开始刷新MCP服务能力...")

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

	global.Log.Info("MCP服务能力刷新完成")
	return nil
}
