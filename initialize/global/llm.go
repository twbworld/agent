package global

import (
	"fmt"
	"net/http"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/llm"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
)

// 初始化LLM服务
func initLlm() error {
	llmClients := make(map[enum.LlmSize]*openai.Client, len(global.Config.Llm))
	for _, cfg := range global.Config.Llm {
		config := openai.DefaultConfig(cfg.Auth)
		config.BaseURL = cfg.Url
		config.HTTPClient = &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second}
		llmClients[enum.LlmSize(cfg.Size)] = openai.NewClientWithConfig(config)
	}

	if len(llmClients) == 0 {
		return fmt.Errorf("未配置任何LLM客户端")
	}

	global.LlmService = llm.NewClient(
		global.Log,
		llmClients,
		global.Config.Llm,
	)
	return nil
}
