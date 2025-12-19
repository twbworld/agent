package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/service/admin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"
)

type LlmService interface {
	// 分诊, 使用小型LLM对用户输入进行分诊，返回分类结果
	Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error)
	// GenerateResponseOrToolCall 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt，并生成初步回复或工具调用指令
	GenerateResponseOrToolCall(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (string, error)
	// ExecuteToolCalls 解析LLM回复中的工具调用指令，并发执行MCP工具，并返回格式化后的工具消息列表
	ExecuteToolCalls(ctx context.Context, llmAnswer string, sender common.Sender) ([]common.LlmMessage, error)
	// SynthesizeToolResult 在工具调用后，综合所有信息（包括工具结果）生成最终的自然语言回复, 不需要知识库(向量)数据了
	SynthesizeToolResult(ctx context.Context, history []common.LlmMessage) (string, error)
}

type llmService struct {
}

func NewLlmService() *llmService {
	return &llmService{}
}

func (s *llmService) Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error) {
	if global.LlmService == nil {
		return nil, fmt.Errorf("LLM客户端未初始化")
	}

	// 构建发送给小模型的prompt
	var prompt strings.Builder

	if len(history) > 0 {
		prompt.WriteString("最近的对话历史:\n")
		for _, msg := range history {
			// 为保证prompt简洁，只显示最核心信息
			fmt.Fprintf(&prompt, "- %s: %s\n", msg.Role, msg.Content)
		}
		prompt.WriteString("\n")
	}

	contextPrompt, err := s.buildContextPrompt(sender)
	if err != nil {
		global.Log.Warnf("[Triage] 构建上下文提示词失败: %v", err)
		// 不中断流程，继续执行
	} else if contextPrompt != "" {
		prompt.WriteString("上下文信息:\n")
		prompt.WriteString(contextPrompt)
		prompt.WriteString("\n")
	}

	fmt.Fprintf(&prompt, "用户最新问题:\n\"%s\"\n\n", content)

	if len(retrievedQuestions) > 0 {
		prompt.WriteString("根据用户的提问，我们在知识库中检索到以下可能相关的问题：\n")
		for i, q := range retrievedQuestions {
			fmt.Fprintf(&prompt, "%d. \"%s\"\n", i+1, q)
		}
		prompt.WriteString("\n")
	}
	prompt.WriteString("请结合以上所有信息进行综合判断。")

	// 使用小模型和专用的Triage Prompt
	triageResultJSON, err := global.LlmService.GetCompletion(ctx, enum.ModelSmall, enum.SystemPromptTriage, prompt.String(), 0.2)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("分诊台LLM调用失败: %w", err)
	}

	var triageResult common.TriageResult
	// 尝试从LLM可能返回的Markdown代码块中提取纯JSON
	cleanJSON := strings.TrimSpace(triageResultJSON)
	if strings.HasPrefix(cleanJSON, "```json") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
		cleanJSON = strings.TrimSpace(cleanJSON)
	}

	if err := json.Unmarshal([]byte(cleanJSON), &triageResult); err != nil {
		return nil, fmt.Errorf("解析分诊台返回的JSON失败: %w, 原始返回: %s", err, triageResultJSON)
	}

	return &triageResult, nil
}

func (s *llmService) GenerateResponseOrToolCall(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (string, error) {
	if global.LlmService == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	hasTools := global.McpService != nil && len(global.McpService.GetAvailableTools()) > 0
	hasDocs := len(referenceDocs) > 0

	var systemPromptBuilder strings.Builder
	var finalContent strings.Builder

	// 1. 根据场景动态构建System Prompt
	// 使用 if-else 选择基础Prompt，避免重复
	if hasDocs {
		systemPromptBuilder.WriteString(string(enum.SystemPromptRAG))
	} else {
		systemPromptBuilder.WriteString(string(enum.SystemPromptDefault))
	}

	// 2. 如果有可用工具，则追加工具使用说明
	if hasTools {
		// 从 enum 中获取工具使用的模板
		toolPromptTemplate := string(enum.SystemPromptToolUser)

		// 构建可用工具列表的字符串
		var toolsListBuilder strings.Builder
		availableClients := global.McpService.GetAvailableToolsWithClient()
		for clientName, tools := range availableClients {
			for _, tool := range tools {
				var argsSchema string
				if tool.InputSchema != nil {
					schemaBytes, err := json.Marshal(tool.InputSchema)
					if err == nil {
						argsSchema = string(schemaBytes)
					}
				}

				if argsSchema != "" {
					toolsListBuilder.WriteString(fmt.Sprintf("- %s.%s: %s. Arguments: %s\n", clientName, tool.Name, tool.Description, argsSchema))
				} else {
					toolsListBuilder.WriteString(fmt.Sprintf("- %s.%s: %s\n", clientName, tool.Name, tool.Description))
				}
			}
		}

		// 将工具列表替换到模板中
		finalToolPrompt := strings.Replace(toolPromptTemplate, "{tools}", toolsListBuilder.String(), 1)
		systemPromptBuilder.WriteString("\n\n") // 添加换行符以分隔
		systemPromptBuilder.WriteString(finalToolPrompt)
	}

	// 3. 构建最终发送给LLM的 content
	// 将 sender 信息作为上下文注入
	contextPrompt, err := s.buildContextPrompt(sender)
	if err != nil {
		global.Log.Warnf("[GenerateResponseOrToolCall] 构建上下文提示词失败: %v", err)
		// 不中断流程，继续执行
	} else if contextPrompt != "" {
		finalContent.WriteString("上下文信息:\n")
		finalContent.WriteString(contextPrompt)
		finalContent.WriteString("\n")
	}

	if hasDocs {
		finalContent.WriteString("--- 参考资料 ---\n")
		for _, doc := range referenceDocs {
			// 确保问题和答案不为空
			q := doc.Question
			if q == "" {
				q = "相关信息"
			}
			fmt.Fprintf(&finalContent, "[问题]: %s\n[回答]: %s\n---\n", q, doc.Answer)
		}
	}

	if finalContent.Len() > 0 {
		finalContent.WriteString("\n")
	}

	finalContent.WriteString("--- 当前系统时间 ---\n")
	finalContent.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	finalContent.WriteString("\n\n")

	finalContent.WriteString("--- 用户问题 ---\n")
	finalContent.WriteString(param.Content)

	return global.LlmService.ChatCompletionWithHistory(
		ctx,
		enum.ModelLarge,
		enum.SystemPrompt(systemPromptBuilder.String()),
		finalContent.String(),
		history,
		0.5,
	)
}

// ExecuteToolCalls 解析并执行工具调用
func (s *llmService) ExecuteToolCalls(ctx context.Context, llmAnswer string, sender common.Sender) ([]common.LlmMessage, error) {
	if global.McpService == nil {
		return nil, fmt.Errorf("MCP服务未初始化")
	}

	// 1. 解析工具调用指令
	toolCalls, err := s.parseToolCalls(llmAnswer)
	if err != nil {
		global.Log.Errorf("[ExecuteToolCalls] 解析工具调用JSON数组失败: %v", err)
		return []common.LlmMessage{{
			Role:    openai.ChatMessageRoleTool,
			Content: fmt.Sprintf("工具调用格式错误: %v", err),
		}}, nil
	}
	if len(toolCalls) == 0 {
		return nil, nil
	}

	// 2. 准备工具定义的快速查找映射
	allToolsMap := make(map[string]mcp.Tool)
	clients := global.McpService.GetAvailableToolsWithClient()
	for cName, tools := range clients {
		for _, t := range tools {
			allToolsMap[cName+"."+t.Name] = t
		}
	}
	toolDescriptions := global.McpService.GetToolDescriptions()

	// 3. 并发执行工具调用
	var toolResults []common.LlmMessage
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // 限制并发数

	for _, toolCall := range toolCalls {
		// 捕获循环变量
		tc := toolCall
		g.Go(func() error {
			// 执行单个工具逻辑
			msg := s.processSingleToolCall(gCtx, tc, sender, allToolsMap, toolDescriptions)

			mu.Lock()
			toolResults = append(toolResults, msg)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		global.Log.Errorf("[ExecuteToolCalls] 执行MCP工具组时发生错误: %v", err)
	}

	return toolResults, nil
}

func (s *llmService) SynthesizeToolResult(ctx context.Context, history []common.LlmMessage) (string, error) {
	if global.LlmService == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	return global.LlmService.ChatCompletionWithHistory(
		ctx,
		enum.ModelMedium,
		enum.SystemPromptSynthesizeToolResult,
		"", // content为空，因为所有上下文都在history中
		history,
		0.6,
	)
}

//从 sender 对象构建上下文提示字符串
func (s *llmService) buildContextPrompt(sender common.Sender) (string, error) {
	attrMap, err := customAttributesToMap(sender.CustomAttributes)
	if err != nil {
		return "", fmt.Errorf("转换CustomAttributes为map失败: %w", err)
	}

	// 将 identifier 添加到 map 中以便统一处理
	if sender.Identifier != nil && *sender.Identifier != "" {
		attrMap["user_id"] = *sender.Identifier
	}

	if len(attrMap) == 0 {
		return "", nil
	}

	// 提取并排序 Keys，确保 Prompt 的确定性，从而提高 KV Cache 命中率
	keys := make([]string, 0, len(attrMap))
	for k := range attrMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var attrBuilder strings.Builder
	hasContent := false
	for _, key := range keys {
		value := attrMap[key]
		if value == nil {
			continue
		}
		if vStr, ok := value.(string); ok && vStr != "" {
			attrBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, vStr))
			hasContent = true
		} else if _, ok := value.(string); !ok {
			attrBuilder.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
			hasContent = true
		}
	}

	if !hasContent {
		return "", nil
	}

	var prompt strings.Builder
	prompt.WriteString("上下文信息:\n")
	prompt.WriteString(attrBuilder.String())
	prompt.WriteString("\n")

	return prompt.String(), nil
}

func customAttributesToMap(attrs common.CustomAttributes) (map[string]interface{}, error) {
	var attrMap map[string]interface{}
	bytes, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &attrMap)
	if err != nil {
		return nil, err
	}
	return attrMap, nil
}



// processSingleToolCall 处理单个工具调用的完整生命周期
func (s *llmService) processSingleToolCall(ctx context.Context, toolCall common.ToolCallParams, sender common.Sender, toolsMap map[string]mcp.Tool, descMap map[string]string) common.LlmMessage {
	var resultStr string
	parts := strings.SplitN(toolCall.Name, ".", 2)

	// 1. 校验名称格式
	if len(parts) != 2 {
		errMsg := fmt.Sprintf("工具名称格式错误，必须为 '客户端名称.工具名称'，实际为: '%s'", toolCall.Name)
		global.Log.Errorf("[ExecuteToolCalls] %s", errMsg)
		return s.formatToolMessage(toolCall.Name, descMap, errMsg)
	}

	clientName, toolName := parts[0], parts[1]

	// 2. 参数安全注入与权限校验
	safeArgs, shouldExecute, rejectReason := s.ensureSafeArguments(toolCall, sender, toolsMap)
	if !shouldExecute {
		return s.formatToolMessage(toolCall.Name, descMap, rejectReason)
	}

	// 3. 执行工具
	global.Log.Debugln(string(safeArgs), "得到工具结果===============")
	rawResult, err := global.McpService.ExecuteTool(ctx, clientName, toolName, safeArgs)
	if err != nil {
		errMsg := fmt.Sprintf("工具 '%s' 调用失败: %v", toolCall.Name, err)
		global.Log.Errorf("[ExecuteToolCalls] %s", errMsg)
		return s.formatToolMessage(toolCall.Name, descMap, errMsg)
	}

	// 4. 结果归属权校验 (IDOR 防护)
	if !s.verifyToolResultOwnership(rawResult, sender) {
		currentUserID := ""
		if sender.Identifier != nil {
			currentUserID = *sender.Identifier
		}
		warnMsg := fmt.Sprintf("安全拦截: 拒绝访问。数据归属与当前用户(%s)不一致。", currentUserID)
		global.Log.Warnf("[ExecuteToolCalls] IDOR拦截: 工具=%s, User=%s", toolCall.Name, currentUserID)
		return s.formatToolMessage(toolCall.Name, descMap, warnMsg)
	}

	resultStr = rawResult
	global.Log.Debugf("=================成功获取Mcp数据 for '%s': %s", toolCall.Name, resultStr)

	return s.formatToolMessage(toolCall.Name, descMap, resultStr)
}

// ensureSafeArguments 检查并注入必要的安全参数(如user_id)，返回处理后的参数和是否允许执行
func (s *llmService) ensureSafeArguments(toolCall common.ToolCallParams, sender common.Sender, toolsMap map[string]mcp.Tool) (json.RawMessage, bool, string) {
	toolDef, exists := toolsMap[toolCall.Name]
	if !exists {
		// 如果工具未定义在映射中，不做Schema检查，直接放行(或根据策略处理，此处保持原逻辑放行)
		return toolCall.Arguments, true, ""
	}

	// 解析当前参数
	var argMap map[string]interface{}
	if err := json.Unmarshal(toolCall.Arguments, &argMap); err != nil {
		// 参数格式错误，但交给后续流程处理或直接尝试执行
		if argMap == nil {
			argMap = make(map[string]interface{})
		}
	}

	// 解析工具Schema以检查是否需要注入user_id
	// 注意: 这种反射/Marshal方式虽然性能一般，但为了保持原逻辑的动态性暂时保留
	var schemaMap map[string]interface{}
	b, _ := json.Marshal(toolDef.InputSchema)
	_ = json.Unmarshal(b, &schemaMap)

	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		return toolCall.Arguments, true, ""
	}

	// 检查是否包含 user_id 字段定义
	if _, hasUser := props[admin.McpArgUserId]; hasUser {
		var safeUserID string
		if sender.Identifier != nil {
			safeUserID = *sender.Identifier
		}

		// 拦截匿名用户
		if safeUserID == "" {
			global.Log.Warnf("[ExecuteToolCalls] 拦截匿名用户调用敏感工具: %s", toolCall.Name)
			return nil, false, "执行失败: 当前用户未登录(无身份标识)，无法执行涉及用户数据的操作。"
		}

		// 强制注入/覆盖 user_id
		argMap[admin.McpArgUserId] = safeUserID
		if newArgs, err := json.Marshal(argMap); err == nil {
			return newArgs, true, ""
		}
	}

	return toolCall.Arguments, true, ""
}

// verifyToolResultOwnership 检查工具返回的数据是否属于当前用户
func (s *llmService) verifyToolResultOwnership(rawResult string, sender common.Sender) bool {
	// 尝试解析JSON结果
	var resultData map[string]interface{}
	if json.Unmarshal([]byte(rawResult), &resultData) != nil {
		// 非JSON结果，默认安全(或无法判断归属)，放行
		return true
	}

	// 提取数据中的用户ID
	var dataUserID string
	if v, ok := resultData[admin.McpArgUserId]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	} else if v, ok := resultData["userId"]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	} else if v, ok := resultData["uid"]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	}

	// 如果数据中没有包含用户ID，则认为不涉及敏感归属，放行
	if dataUserID == "" {
		return true
	}

	// 比较当前用户ID
	currentUserID := ""
	if sender.Identifier != nil {
		currentUserID = *sender.Identifier
	}

	return dataUserID == currentUserID
}

// formatToolMessage 格式化工具返回的消息，并进行截断处理
func (s *llmService) formatToolMessage(toolName string, descMap map[string]string, content string) common.LlmMessage {
	// 截断结果
	const maxToolResultLength = 2048
	runes := []rune(content)
	if len(runes) > maxToolResultLength {
		content = string(runes[:maxToolResultLength]) + "\n...(内容过长已截断)"
	}

	toolDescription := "未知工具"
	if desc, ok := descMap[toolName]; ok {
		toolDescription = desc
	}

	finalContent := fmt.Sprintf(
		"[工具名称]: %s\n[工具作用]: %s\n[返回结果]:\n%s",
		toolName,
		toolDescription,
		content,
	)

	return common.LlmMessage{
		Role:    openai.ChatMessageRoleTool,
		Content: finalContent,
	}
}

// parseToolCalls 提取并解析工具调用JSON
func (s *llmService) parseToolCalls(llmAnswer string) (common.ToolCalls, error) {
	startIdx := strings.Index(llmAnswer, "<tool_code>")
	endIdx := strings.Index(llmAnswer, "</tool_code>")
	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("未找到完整的工具调用标签")
	}

	// +len("<tool_code>") 跳过标签本身
	jsonContent := strings.TrimSpace(llmAnswer[startIdx+11 : endIdx])

	// 处理 LLM 可能在 XML 标签内部再次包裹 Markdown 代码块的情况
	if strings.HasPrefix(jsonContent, "```json") {
		jsonContent = strings.TrimPrefix(jsonContent, "```json")
		jsonContent = strings.TrimSuffix(jsonContent, "```")
		jsonContent = strings.TrimSpace(jsonContent)
	} else if strings.HasPrefix(jsonContent, "```") {
		jsonContent = strings.TrimPrefix(jsonContent, "```")
		jsonContent = strings.TrimSuffix(jsonContent, "```")
		jsonContent = strings.TrimSpace(jsonContent)
	}

	var toolCalls common.ToolCalls
	if err := json.Unmarshal([]byte(jsonContent), &toolCalls); err != nil {
		return nil, err
	}
	return toolCalls, nil
}
