package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"
)

type LlmService interface {
	// 分诊, 使用小型LLM对用户输入进行分诊，返回分类结果
	Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error)
	// GenerateResponseOrToolCall 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt，并生成初步回复或工具调用指令
	GenerateResponseOrToolCall(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (string, error)
	// ExecuteToolCalls 解析LLM回复中的工具调用指令，并发执行MCP工具，并返回格式化后的工具消息列表
	ExecuteToolCalls(ctx context.Context, llmAnswer string) ([]common.LlmMessage, error)
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
func (s *llmService) ExecuteToolCalls(ctx context.Context, llmAnswer string) ([]common.LlmMessage, error) {
	if global.McpService == nil {
		return nil, fmt.Errorf("MCP服务未初始化")
	}

	// 提取JSON内容
	startIdx := strings.Index(llmAnswer, "<tool_code>")
	endIdx := strings.Index(llmAnswer, "</tool_code>")
	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("未找到完整的工具调用标签")
	}

	// +len("<tool_code>") 跳过标签本身
	jsonContent := strings.TrimSpace(llmAnswer[startIdx+11 : endIdx])

	var toolCalls common.ToolCalls
	if err := json.Unmarshal([]byte(jsonContent), &toolCalls); err != nil {
		global.Log.Errorf("[ExecuteToolCalls] 解析工具调用JSON数组失败: %v", err)
		// 解析失败时，将错误信息作为Tool Message返回，让LLM感知到错误
		return []common.LlmMessage{{
			Role:    openai.ChatMessageRoleTool,
			Content: fmt.Sprintf("工具调用格式错误: %v", err),
		}}, nil
	}

	if len(toolCalls) == 0 {
		return nil, nil
	}

	// 获取工具描述以便在结果中增强上下文
	toolDescriptions := global.McpService.GetToolDescriptions()

	// 并发执行
	var toolResults []common.LlmMessage
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5) // 限制并发数

	for _, toolCall := range toolCalls {
		toolCall := toolCall // 避免闭包陷阱
		g.Go(func() error {
			var toolResultContent string
			parts := strings.SplitN(toolCall.Name, ".", 2)

			// 验证工具名称格式
			if len(parts) != 2 {
				toolResultContent = fmt.Sprintf("工具名称格式错误，必须为 '客户端名称.工具名称'，实际为: '%s'", toolCall.Name)
				global.Log.Errorf("[ExecuteToolCalls] %s", toolResultContent)
			} else {
				clientName, toolName := parts[0], parts[1]
				result, err := global.McpService.ExecuteTool(gCtx, clientName, toolName, toolCall.Arguments)
				if err != nil {
					toolResultContent = fmt.Sprintf("工具 '%s' 调用失败: %v", toolCall.Name, err)
					global.Log.Errorf("[ExecuteToolCalls] %s", toolResultContent)
				} else {
					toolResultContent = result
					global.Log.Debugf("=================成功获取Mcp数据 for '%s': %s", toolCall.Name, toolResultContent)
				}
			}

			// 处理数据敏感性: 对工具返回结果进行截断，防止Redis/Context爆满或泄露过多非必要信息
			// 使用runes进行长度判断以支持多语言字符
			const maxToolResultLength = 2048
			runes := []rune(toolResultContent)
			if len(runes) > maxToolResultLength {
				toolResultContent = string(runes[:maxToolResultLength]) + "\n...(内容过长已截断)"
			}

			// 获取工具描述
			toolDescription := "未知工具"
			if desc, ok := toolDescriptions[toolCall.Name]; ok {
				toolDescription = desc
			}

			// 构建结构化的返回消息
			finalContent := fmt.Sprintf(
				"[工具名称]: %s\n[工具作用]: %s\n[返回结果]:\n%s",
				toolCall.Name,
				toolDescription,
				toolResultContent,
			)

			mu.Lock()
			toolResults = append(toolResults, common.LlmMessage{
				Role:    openai.ChatMessageRoleTool,
				Content: finalContent,
			})
			mu.Unlock()
			return nil
		})
	}

	// 等待执行完成
	if err := g.Wait(); err != nil {
		global.Log.Errorf("[ExecuteToolCalls] 执行MCP工具组时发生错误: %v", err)
		// 即使部分失败，也尽量返回已收集到的结果
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

// buildContextPrompt 从 sender 对象构建上下文提示字符串
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

	var attrBuilder strings.Builder
	hasContent := false
	for key, value := range attrMap {
		// 忽略空值
		if value == nil {
			continue
		}
		if vStr, ok := value.(string); ok && vStr != "" {
			attrBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, vStr))
			hasContent = true
		} else if _, ok := value.(string); !ok { // 处理非字符串类型
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

// customAttributesToMap 使用JSON序列化和反序列化将CustomAttributes结构体安全地转换为map
func customAttributesToMap(attrs common.CustomAttributes) (map[string]interface{}, error) {
	var attrMap map[string]interface{}
	// 通过JSON序列化和反序列化来转换
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
