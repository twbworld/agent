package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
)

type LlmService interface {
	// 分诊, 使用小型LLM对用户输入进行分诊，返回分类结果
	Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error)
	// GenerateResponseOrToolCall 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt，并生成初步回复或工具调用指令
	GenerateResponseOrToolCall(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (string, error)
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
	}
	if contextPrompt != "" {
		prompt.WriteString(contextPrompt)
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
	}
	if contextPrompt != "" {
		finalContent.WriteString(contextPrompt)
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

func (s *llmService) SynthesizeToolResult(ctx context.Context, history []common.LlmMessage) (string, error) {
	if global.LlmService == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	// 在这个阶段，我们使用一个干净、简单的系统提示，因为LLM的任务只是根据现有对话（包括工具结果）进行总结。
	// 无需再次提供复杂的RAG或工具调用指令。
	return global.LlmService.ChatCompletionWithHistory(
		ctx,
		enum.ModelMedium,
		enum.SystemPromptSynthesizeToolResult, // 使用专用的提示词进行结果合成
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
