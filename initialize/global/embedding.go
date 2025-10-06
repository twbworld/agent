package global

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/embedding"
	"github.com/sashabaranov/go-openai"
)

func initLlmEmbedding() error {
	config := openai.DefaultConfig(global.Config.LlmEmbedding.Auth)
	config.BaseURL = global.Config.LlmEmbedding.Url
	config.HTTPClient = &http.Client{Timeout: time.Duration(global.Config.LlmEmbedding.Timeout) * time.Second}
	openAIClient := openai.NewClientWithConfig(config)

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := openAIClient.ListModels(ctx); err != nil {
		return fmt.Errorf("无法连接到向量化服务 (url: %s): %w", config.BaseURL, err)
	}

	global.EmbeddingService = embedding.NewClient(
		openAIClient,
		global.Config.LlmEmbedding.Model,
	)
	return nil
}
