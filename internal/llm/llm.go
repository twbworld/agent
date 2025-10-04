package llm

import (
	"context"
	"errors"
	"strings"

	"gitee.com/taoJie_1/chat/model/config"
	"gitee.com/taoJie_1/chat/model/enum"
	"gitee.com/taoJie_1/chat/pkg/llm"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// client 封装了与LLM交互的底层逻辑
type client struct {
	log        *logrus.Logger
	llmClients map[enum.LlmSize]*openai.Client
	llmConfigs []config.Llm
}

// NewClient 创建一个新的LLM客户端实例，并通过依赖注入初始化
func NewClient(log *logrus.Logger, clients map[enum.LlmSize]*openai.Client, configs []config.Llm) llm.Service {
	return &client{
		log:        log,
		llmClients: clients,
		llmConfigs: configs,
	}
}

// getModelName 是一个内部辅助函数，用于根据大小获取模型名称
func (c *client) getModelName(size enum.LlmSize) string {
	for _, cfg := range c.llmConfigs {
		if enum.LlmSize(cfg.Size) == size {
			return cfg.Model
		}
	}
	// 如果没找到（例如 medium 降级到了 small），则使用 small 的模型名
	return c.llmConfigs[0].Model
}

// ChatCompletion 调用LLM进行对话
// ctx: 请求上下文，用于处理如用户取消请求等情况
// size: 要使用的模型大小 (small, large)
// prompt: 系统提示词
// content: 用户输入的内容
func (c *client) ChatCompletion(ctx context.Context, size enum.LlmSize, prompt enum.SystemPrompt, content string) (string, error) {
	modelName := c.getModelName(size)

	req := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: string(prompt),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: content,
			},
		},
	}

	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	resp, err := llmClient.CreateChatCompletion(ctx, req)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", errors.New("请求已被用户取消")
		}
		c.log.Errorf("LLM API调用失败: %v", err)
		return "", errors.New("LLM服务暂不可用, 请稍后再试")
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", errors.New("LLM服务返回了空结果")
	}
	llmAnswer := resp.Choices[0].Message.Content

	// 处理回答，去掉可能的思考标记
	var finalAnswer string
	if parts := strings.SplitN(llmAnswer, "</think>", 2); len(parts) > 1 {
		finalAnswer = parts[1]
	} else {
		finalAnswer = parts[0]
	}
	return strings.TrimSpace(finalAnswer), nil
}
