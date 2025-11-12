package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/model/config"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// Service 定义了与MCP服务交互的接口
type Service interface {
	// Close 关闭所有MCP会话
	Close() error
	// GetAvailableTools 返回所有已连接的MCP服务提供的工具列表
	GetAvailableTools() []mcp.Tool
	// GetAvailableToolsWithClient 返回按客户端名称分组的工具列表
	GetAvailableToolsWithClient() map[string][]mcp.Tool
	// GetToolDescriptions 返回一个从工具全名到其描述的映射
	GetToolDescriptions() map[string]string
	// ExecuteTool 解析并执行来自LLM的工具调用请求
	ExecuteTool(ctx context.Context, clientName string, toolName string, arguments json.RawMessage) (string, error)
	// AddOrUpdateClient 添加或更新一个MCP客户端配置，并执行一次性连接以发现工具
	AddOrUpdateClient(name string, cfg config.Mcp) error
	// RemoveClient 移除一个MCP客户端
	RemoveClient(name string) error
}

// client 是 Service 接口的实现
type client struct {
	log         *logrus.Logger
	clients     map[string]*mcp.Client
	configs     map[string]config.Mcp
	tools       map[string]map[string]mcp.Tool // 按客户端和服务名称存储工具，便于快速查找
	mu          sync.RWMutex
	appVersion  string
	projectName string
}

// transportWithAuth 是一个自定义的 http.RoundTripper，用于在每个请求中添加认证头
type transportWithAuth struct {
	http.RoundTripper
	token string
}

func (t *transportWithAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	// 克隆请求以避免并发问题
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+t.token)
	return t.RoundTripper.RoundTrip(req2)
}

// NewClient 创建并初始化一个新的MCP服务客户端
func NewClient(log *logrus.Logger, mcpConfigs map[string]config.Mcp, appVersion, projectName string) (Service, error) {
	c := &client{
		log:         log,
		clients:     make(map[string]*mcp.Client),
		configs:     make(map[string]config.Mcp),
		tools:       make(map[string]map[string]mcp.Tool),
		appVersion:  appVersion,
		projectName: projectName,
	}

	for name, cfg := range mcpConfigs {
		if err := c.AddOrUpdateClient(name, cfg); err != nil {
			log.Errorf("初始化MCP客户端 '%s' 失败: %v", name, err)
			// 即使某个客户端失败，也继续尝试其他客户端
		}
	}

	return c, nil
}

func (c *client) AddOrUpdateClient(name string, cfg config.Mcp) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 更新客户端和配置缓存
	c.clients[name] = mcp.NewClient(&mcp.Implementation{Name: c.projectName, Version: c.appVersion}, nil)
	c.configs[name] = cfg

	// 为一次性工具发现操作配置transport
	httpClient := &http.Client{
		Transport: &transportWithAuth{
			RoundTripper: http.DefaultTransport,
			token:        cfg.Auth,
		},
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   cfg.Url,
		HTTPClient: httpClient,
	}

	// 使用带超时的上下文执行一次性连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := c.clients[name].Connect(ctx, transport, nil)
	if err != nil {
		c.log.Errorf("为MCP服务 '%s' 发现工具时连接失败: %v", name, err)
		delete(c.tools, name) // 确保没有旧的工具信息
		return nil            // 返回nil以避免阻塞其他客户端的初始化
	}
	defer session.Close() // 操作完成立即关闭会话

	// 发现并缓存工具
	loadedTools := make(map[string]mcp.Tool)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			c.log.Errorf("从MCP服务 '%s' 获取工具列表时出错: %v", name, err)
			delete(c.tools, name)
			return nil // 返回nil
		}
		loadedTools[tool.Name] = *tool
	}

	c.tools[name] = loadedTools
	c.log.Infof("成功为MCP服务 '%s' 发现 %d 个工具", name, len(loadedTools))
	return nil
}

func (c *client) RemoveClient(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clients, name)
	delete(c.configs, name)
	delete(c.tools, name)
	c.log.Infof("已移除MCP客户端: %s", name)
	return nil
}

// removeClientUnderLock 是一个内部函数，在持有锁的情况下移除客户端
func (c *client) removeClientUnderLock(name string) {
	delete(c.clients, name)
	delete(c.configs, name)
	delete(c.tools, name)
}

func (c *client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.clients {
		c.removeClientUnderLock(name)
	}
	return nil
}

func (c *client) GetAvailableTools() []mcp.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var allTools []mcp.Tool
	for _, clientTools := range c.tools {
		for _, tool := range clientTools {
			allTools = append(allTools, tool)
		}
	}
	return allTools
}

func (c *client) GetAvailableToolsWithClient() map[string][]mcp.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 返回一个副本以保证并发安全
	toolsCopy := make(map[string][]mcp.Tool, len(c.tools))
	for clientName, tools := range c.tools {
		clientToolsList := make([]mcp.Tool, 0, len(tools))
		for _, tool := range tools {
			clientToolsList = append(clientToolsList, tool)
		}
		toolsCopy[clientName] = clientToolsList
	}
	return toolsCopy
}

func (c *client) GetToolDescriptions() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	descriptions := make(map[string]string)
	for clientName, clientTools := range c.tools {
		for toolName, tool := range clientTools {
			fullName := fmt.Sprintf("%s.%s", clientName, toolName)
			descriptions[fullName] = tool.Description
		}
	}
	return descriptions
}

// coerceArguments 尝试根据工具的 schema 转换参数类型。
// 例如，如果 schema 要求一个整数，它会将字符串 "123" 转换为数字 123。
func (c *client) coerceArguments(arguments json.RawMessage, schema *jsonschema.Schema) (json.RawMessage, error) {
	if len(arguments) == 0 || string(arguments) == "null" {
		return arguments, nil
	}

	var argsMap map[string]interface{}
	if err := json.Unmarshal(arguments, &argsMap); err != nil {
		return nil, fmt.Errorf("无法将参数解码为map: %w", err)
	}

	if schema.Properties == nil {
		return arguments, nil // 没有属性可供校验
	}

	for key, value := range argsMap {
		propSchema, ok := schema.Properties[key]
		if !ok {
			continue // 此属性没有对应的 schema
		}

		// go-sdk 中的 jsonschema.Schema 使用单个 `Type` 字段
		expectedType := propSchema.Type
		if expectedType == "" {
			continue
		}

		// 只处理从 string 到其他基本类型的转换，这是 LLM 最常见的错误
		if valStr, ok := value.(string); ok {
			switch expectedType {
			case "integer":
				if intVal, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					argsMap[key] = intVal
				}
			case "number":
				if floatVal, err := strconv.ParseFloat(valStr, 64); err == nil {
					argsMap[key] = floatVal
				}
			case "boolean":
				if boolVal, err := strconv.ParseBool(valStr); err == nil {
					argsMap[key] = boolVal
				}
			}
		}
	}

	coercedJSON, err := json.Marshal(argsMap)
	if err != nil {
		return nil, fmt.Errorf("无法将修正后的参数重新编码为JSON: %w", err)
	}

	return coercedJSON, nil
}

func (c *client) ExecuteTool(ctx context.Context, clientName string, toolName string, arguments json.RawMessage) (string, error) {
	c.mu.RLock()
	cfg, cfgOk := c.configs[clientName]
	mcpClient, clientOk := c.clients[clientName]
	clientTools, toolsOk := c.tools[clientName]
	var tool mcp.Tool
	var toolOk bool
	if toolsOk {
		tool, toolOk = clientTools[toolName]
	}
	c.mu.RUnlock()

	if !cfgOk {
		return "", fmt.Errorf("未找到名为 '%s' 的MCP客户端配置", clientName)
	}
	if !clientOk {
		return "", fmt.Errorf("未找到名为 '%s' 的MCP逻辑客户端", clientName)
	}

	// 对LLM生成的参数进行类型矫正
	finalArguments := arguments
	if toolOk && tool.InputSchema != nil {
		schemaBytes, err := json.Marshal(tool.InputSchema)
		if err != nil {
			c.log.Warnf("无法序列化工具 '%s' 的InputSchema: %v。将尝试使用原始参数。", toolName, err)
		} else {
			var inputSchema jsonschema.Schema
			if err := json.Unmarshal(schemaBytes, &inputSchema); err != nil {
				c.log.Warnf("无法反序列化工具 '%s' 的InputSchema为jsonschema.Schema: %v。将尝试使用原始参数。", toolName, err)
			} else {
				correctedArgs, err := c.coerceArguments(arguments, &inputSchema)
				if err != nil {
					c.log.Warnf("MCP工具 '%s' 的参数类型转换失败: %v。将尝试使用原始参数。", toolName, err)
				} else {
					finalArguments = correctedArgs
				}
			}
		}
	}

	// 为本次调用配置transport
	httpClient := &http.Client{
		Transport: &transportWithAuth{
			RoundTripper: http.DefaultTransport,
			token:        cfg.Auth,
		},
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   cfg.Url,
		HTTPClient: httpClient,
	}

	// 按需执行 连接-调用-关闭
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return "", fmt.Errorf("执行工具时连接到MCP服务 '%s' 失败: %w", clientName, err)
	}
	defer session.Close()

	params := mcp.CallToolParams{
		Name:      toolName,
		Arguments: finalArguments,
	}

	res, err := session.CallTool(ctx, &params)
	if err != nil {
		return "", fmt.Errorf("调用工具 '%s' 失败: %w", params.Name, err)
	}

	if res.IsError {
		var errorContent strings.Builder
		for _, content := range res.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				errorContent.WriteString(textContent.Text)
			}
		}
		return "", fmt.Errorf("工具 '%s' 执行返回错误: %s", params.Name, errorContent.String())
	}

	var resultBuilder strings.Builder
	for _, content := range res.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			resultBuilder.WriteString(textContent.Text)
		}
	}
	return resultBuilder.String(), nil
}
