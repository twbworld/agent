package user

import (
	"context"
	"fmt"
	"strings"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/enum"
)

type LlmService interface {
	// 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt
	NewChat(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage) (string, error) // 修改方法签名
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
