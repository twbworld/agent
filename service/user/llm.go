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
	// 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt
	NewChat(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage) (string, error)
	// 使用小型LLM对用户输入进行分诊，返回分类结果
	Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string) (*common.TriageResult, error)
}

type llmService struct {
}

func NewLlmService() *llmService {
	return &llmService{}
}

func (s *llmService) NewChat(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage) (string, error) { // 修改方法签名
	if global.LlmService == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	var finalContent strings.Builder
	var systemPrompt enum.SystemPrompt = enum.SystemPromptDefault

	// 如果有参考文档，则构建一个包含上下文的prompt
	if len(referenceDocs) > 0 {
		systemPrompt = enum.SystemPromptRAG
		finalContent.WriteString("--- 参考资料 ---\n")
		for _, doc := range referenceDocs {
			fmt.Fprintf(&finalContent, "[问题]: %s\n[回答]: %s\n---\n", doc.Question, doc.Answer)
		}
		finalContent.WriteString("\n--- 用户问题 ---\n")
	}

	finalContent.WriteString(param.Content)

	return global.LlmService.ChatCompletionWithHistory(
		ctx,
		enum.ModelLarge,
		systemPrompt,
		finalContent.String(),
		history,
		0.5,
	)
}

func (s *llmService) Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string) (*common.TriageResult, error) {
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
