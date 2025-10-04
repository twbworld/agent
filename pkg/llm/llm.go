package llm

import (
	"context"
	"gitee.com/taoJie_1/chat/model/enum"
)

type Service interface {
	ChatCompletion(ctx context.Context, size enum.LlmSize, prompt enum.SystemPrompt, content string) (string, error)
}
