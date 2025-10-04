package global

import (
	"fmt"
	"net/http"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/embedding"
	"github.com/sashabaranov/go-openai"
)

func initLlmEmbedding() error {
	if global.Config.LlmEmbedding.Auth == "" {
		return fmt.Errorf("未配置LLM Embedding API Key")
	}

	config := openai.DefaultConfig(global.Config.LlmEmbedding.Auth)
	config.BaseURL = global.Config.LlmEmbedding.Url
	config.HTTPClient = &http.Client{Timeout: time.Duration(global.Config.LlmEmbedding.Timeout) * time.Second}
	openAIClient := openai.NewClientWithConfig(config)

	global.EmbeddingService = embedding.NewClient(
		openAIClient,
		global.Config.LlmEmbedding.Model,
	)
	return nil
}
