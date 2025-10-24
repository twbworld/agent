package embedding

import (
	"context"
	"fmt"
	"github.com/sashabaranov/go-openai"
)

type client struct {
	openAIClient *openai.Client
	modelName    string
}

type Service interface {
	// 批量将多个文本转换为向量
	CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}

func NewClient(openAIClient *openai.Client, modelName string) Service {
	return &client{
		openAIClient: openAIClient,
		modelName:    modelName,
	}
}

func (c *client) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	req := openai.EmbeddingRequest{
		Input: texts,
		Model: openai.EmbeddingModel(c.modelName),
	}

	resp, err := c.openAIClient.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("请求LLM向量化错误: %w", err)
	}

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("向量数据不匹配: expected %d, got %d", len(texts), len(resp.Data))
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, data := range resp.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}
