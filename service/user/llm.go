package user

import (
	"context"
	"fmt"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
)

type ILlmService interface {
	NewChat(ctx context.Context, param *common.ChatRequest) (string, error)
}

type LlmService struct {
}

func NewLlmService() *LlmService {
	return &LlmService{}
}

// 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt
func (s *LlmService) NewChat(ctx context.Context, param *common.ChatRequest) (string, error) {
	if global.LlmService == nil {
		return  "", fmt.Errorf("LLM客户端未初始化")
	}

	return global.LlmService.ChatCompletion(
		ctx,
		enum.ModelLarge,
		enum.SystemPromptDefault,
		param.Content,
		0.5,
	)
}
