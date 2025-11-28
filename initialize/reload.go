package initialize

import (
	"context"
	"reflect"
	"strings"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/config"
	"gitee.com/taoJie_1/mall-agent/service"
	"gitee.com/taoJie_1/mall-agent/service/user"
	"golang.org/x/sync/errgroup"
)

// HandleConfigChange 检测配置变化并安全地、并发地重载相关服务
func (i *Initializer) HandleConfigChange(oldConfig, newConfig *config.Config) {
	i.reloadLock.Lock()
	defer i.reloadLock.Unlock()

	var restartNeeded []string

	// --- 1. 检查不可热重载的高风险配置 ---
	if !reflect.DeepEqual(oldConfig.Database, newConfig.Database) {
		restartNeeded = append(restartNeeded, "database")
	}
	if oldConfig.GinAddr != newConfig.GinAddr {
		restartNeeded = append(restartNeeded, "gin_addr")
	}
	if oldConfig.GinLogPath != newConfig.GinLogPath || oldConfig.RunLogPath != newConfig.RunLogPath {
		restartNeeded = append(restartNeeded, "log_path")
	}

	// --- 2. 并发执行可安全热重载的任务 ---
	eg, _ := errgroup.WithContext(context.Background())

	// 时区重载
	if oldConfig.Tz != newConfig.Tz {
		eg.Go(func() error {
			if err := i.InitTz(); err != nil {
				global.Log.Errorf("热重载时区失败: %v", err)
				return err
			}
			return nil
		})
	}

	// Redis客户端重载
	if !reflect.DeepEqual(oldConfig.Redis, newConfig.Redis) {
		eg.Go(func() error {
			if err := i.redisClose(); err != nil {
				global.Log.Warnf("关闭旧Redis客户端失败: %v", err)
			}
			if err := i.initRedis(); err != nil {
				global.Log.Errorf("热重载Redis客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// Chatwoot客户端重载
	if !reflect.DeepEqual(oldConfig.Chatwoot, newConfig.Chatwoot) {
		eg.Go(func() error {
			if err := i.initChatwoot(); err != nil {
				global.Log.Errorf("热重载Chatwoot客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// LLM服务重载
	if !reflect.DeepEqual(oldConfig.Llm, newConfig.Llm) {
		eg.Go(func() error {
			if err := i.initLlm(); err != nil {
				global.Log.Errorf("热重载LLM服务失败: %v", err)
				return err
			}
			return nil
		})
	}

	// 向量化模型服务重载
	if !reflect.DeepEqual(oldConfig.LlmEmbedding, newConfig.LlmEmbedding) {
		eg.Go(func() error {
			if err := i.initLlmEmbedding(); err != nil {
				global.Log.Errorf("热重载向量化模型服务失败: %v", err)
				return err
			}
			return nil
		})
	}

	// 向量数据库客户端重载
	if !reflect.DeepEqual(oldConfig.VectorDb, newConfig.VectorDb) {
		eg.Go(func() error {
			if err := i.vectorDbClose(); err != nil {
				global.Log.Warnf("关闭旧向量数据库客户端失败: %v", err)
			}
			if err := i.initVectorDb(); err != nil {
				global.Log.Errorf("热重载向量数据库客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	// 数据库服务重载 (高风险操作，请谨慎使用)
	if !reflect.DeepEqual(oldConfig.Database, newConfig.Database) {
		// eg.Go(func() error {
		// 	global.Log.Warn("检测到数据库配置变更，将执行高风险的热重载操作...")
		// 	if err := i.dbClose(); err != nil {
		// 		// 此处错误可能是非致命的（例如旧连接已失效），记录警告即可
		// 		global.Log.Warnf("关闭旧数据库连接失败: %v", err)
		// 	}
		// 	if err := i.dbStart(); err != nil {
		// 		global.Log.Errorf("热重载数据库失败: %v", err)
		// 		// 数据库重载失败是严重问题，应中断并返回错误
		// 		return err
		// 	}
		// 	return nil
		// })
	}

	// AI相关业务逻辑配置重载
	if !reflect.DeepEqual(oldConfig.Ai, newConfig.Ai) {
		eg.Go(func() error {
			// ActionService依赖于Ai.TransferKeywords，需要重新初始化
			service.Service.UserServiceGroup = user.NewServiceGroup(i.taskManager)
			return nil
		})
	}

	// MCP服务重载
	if !reflect.DeepEqual(oldConfig.McpServers, newConfig.McpServers) {
		eg.Go(func() error {
			if global.McpService == nil {
				// 如果之前未初始化，则进行初始化
				if err := i.initMcp(); err != nil {
					global.Log.Errorf("热重载期间初始化MCP服务失败: %v", err)
					return err
				}
				return nil
			}

			oldMap := oldConfig.McpServers
			newMap := newConfig.McpServers

			for name, oldCfg := range oldMap {
				if newCfg, ok := newMap[name]; !ok {
					// 被移除
					global.McpService.RemoveClient(name)
				} else if !reflect.DeepEqual(oldCfg, newCfg) {
					// 被修改
					global.McpService.AddOrUpdateClient(name, newCfg)
				}
			}

			// 新增的
			for name, newCfg := range newMap {
				if _, ok := oldMap[name]; !ok {
					global.McpService.AddOrUpdateClient(name, newCfg)
				}
			}
			return nil
		})
	}

	// OSS 服务重载
	if !reflect.DeepEqual(oldConfig.Oss, newConfig.Oss) {
		eg.Go(func() error {
			if err := i.ossClose(); err != nil {
				global.Log.Warnf("关闭旧OSS客户端失败: %v", err)
			}
			if err := i.initOss(); err != nil {
				global.Log.Errorf("热重载OSS客户端失败: %v", err)
				return err
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		global.Log.Errorf("并发热重载过程中发生错误: %v", err)
	}

	// --- 3. 如果有需要重启的变更，发出统一警告 ---
	if len(restartNeeded) > 0 {
		global.Log.Warnf("检测到存在需要 重启服务 才能生效的配置变更: [%s]。", strings.Join(restartNeeded, ", "))
	}

	global.Log.Info("配置变更处理完成")
}
