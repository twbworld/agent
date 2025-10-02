package user

import (
	"context"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/llm"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
)

// LlmService 封装了与LLM相关的业务逻辑
type LlmService struct {
	llmClient *llm.Client // 包含一个底层的llm客户端
}

// NewLlmService 创建一个新的LlmService实例
func NewLlmService() *LlmService {
	return &LlmService{
		// 在这里进行依赖注入，将全局对象传递给底层的llm客户端
		llmClient: llm.NewClient(
			global.Log,
			global.Llm,
			global.Config.Llm,
		),
	}
}

// NewChat 是调用LLM的核心业务方法
// 它现在只负责业务层面的决策，例如决定使用哪个模型、哪个Prompt
// 注意：它的第一个参数现在是标准的 context.Context，实现了与Gin框架的解耦
func (s *LlmService) NewChat(ctx context.Context, param *common.ChatRequest) (string, error) {

	// 业务逻辑：对于普通聊天，我们使用小模型和基础的Prompt
	// 将调用细节委托给llmClient
	// 直接传递传入的上下文，该上下文来自后台goroutine (context.Background())
	return s.llmClient.ChatCompletion(
		ctx,
		enum.ModelSmall,
		enum.PromptNoThink,
		param.Content,
	)
}
