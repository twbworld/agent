package user

import (
	"context"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
)

type LlmService struct {
}

func NewLlmService() *LlmService {
	return &LlmService{
	}
}

// 负责业务层面的决策，例如决定使用哪个模型、哪个Prompt
func (s *LlmService) NewChat(ctx context.Context, param *common.ChatRequest) (string, error) {

	return global.LlmService.ChatCompletion(
		ctx,
		enum.ModelSmall,
		enum.PromptNoThink,
		param.Content,
	)
}
