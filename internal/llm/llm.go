package llm

import (
	"context"
	"errors"
	"strings"

	"gitee.com/taoJie_1/chat/model/config"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// client 封装了与LLM交互的底层逻辑
type client struct {
	log        *logrus.Logger
	llmClients map[enum.LlmSize]*openai.Client
	llmConfigs []config.Llm
}

type Service interface {
	ChatCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string) (string, error)
	GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string) (string, error)
}

// NewClient 创建一个新的LLM客户端实例，并通过依赖注入初始化
func NewClient(log *logrus.Logger, clients map[enum.LlmSize]*openai.Client, configs []config.Llm) Service {
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
	// 如果没找到指定大小的模型，则默认使用第一个配置的模型
	if len(c.llmConfigs) > 0 {
		return c.llmConfigs[0].Model
	}
	return ""
}

// ChatCompletion 调用LLM进行实时对话
func (c *client) ChatCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string) (string, error) {
	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	modelName := c.getModelName(size)
	if modelName == "" {
		return "", errors.New("未找到指定的LLM客户端名称")
	}

	req := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: string(systemPrompt),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: content,
			},
		},
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

// GetCompletion 执行一次性的文本生成任务，通常用于后台任务。
func (c *client) GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string) (string, error) {
	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	modelName := c.getModelName(size)
	if modelName == "" {
		return "", errors.New("未找到指定的LLM客户端名称")
	}

	req := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: string(systemPrompt),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: content,
			},
		},
		Temperature: 0.3, // 温度值越小,输出的稳定性和准确性越高
	}

	resp, err := llmClient.CreateChatCompletion(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", errors.New("LLM请求被取消")
		}
		c.log.Errorf("LLM API(GetCompletion)调用失败: %v", err)
		return "", errors.New("LLM服务(GetCompletion)暂不可用")
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", errors.New("LLM服务(GetCompletion)返回了空结果")
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
