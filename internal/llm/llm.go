package llm

import (
	"context"
	"errors"
	"strings"

	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/config"
	"gitee.com/taoJie_1/mall-agent/model/enum"
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
	// 调用LLM进行实时对话
	ChatCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error)
	// 调用LLM进行实时对话，并支持传入历史消息
	ChatCompletionWithHistory(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, history []common.LlmMessage, temperature ...float32) (string, error)
	// 执行一次性的文本生成任务，通常用于后台任务。
	GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error)
	// 根据输入文本（关键词或内容），使用小模型生成一个标准的、自然的问句
	GenerateStandardQuestion(ctx context.Context, prompt enum.SystemPrompt, text string) (string, error)
}

// NewClient 创建一个新的LLM客户端实例，并通过依赖注入初始化
func NewClient(log *logrus.Logger, clients map[enum.LlmSize]*openai.Client, configs []config.Llm) Service {
	return &client{
		log:        log,
		llmClients: clients,
		llmConfigs: configs,
	}
}

// getLlmConfig 是一个内部辅助函数，用于根据大小获取模型配置
func (c *client) getLlmConfig(size enum.LlmSize) *config.Llm {
	for i := range c.llmConfigs {
		if enum.LlmSize(c.llmConfigs[i].Size) == size {
			return &c.llmConfigs[i]
		}
	}
	// 如果没找到指定大小的模型，则默认使用第一个配置的模型
	if len(c.llmConfigs) > 0 {
		return &c.llmConfigs[0]
	}
	return nil
}

// filterContent 从LLM的原始响应中剥离思考过程标签
func (c *client) filterContent(rawAnswer string) string {
	if parts := strings.SplitN(rawAnswer, "</think>", 2); len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(rawAnswer)
}

func (c *client) ChatCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error) {
	return c.ChatCompletionWithHistory(ctx, size, systemPrompt, content, nil, temperature...)
}

// systemPrompt: LLM的系统提示词
// content: 用户问题 + 知识库参考资料 (RAG)
// history: 之前的对话历史消息列表
func (c *client) ChatCompletionWithHistory(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, history []common.LlmMessage, temperature ...float32) (string, error) {
	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	llmConfig := c.getLlmConfig(size)
	if llmConfig == nil || llmConfig.Model == "" {
		return "", errors.New("未找到指定的LLM客户端配置")
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: string(systemPrompt),
		},
	}

	// 添加历史消息
	for _, msg := range history {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 添加当前用户消息
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: content,
	})

	req := openai.ChatCompletionRequest{
		Model:    llmConfig.Model,
		Messages: messages,
	}

	// 优先使用传入的temperature参数，其次是配置文件中的，最后使用LLM默认值
	if len(temperature) > 0 {
		req.Temperature = temperature[0]
	} else if llmConfig.Temperature != nil {
		req.Temperature = *llmConfig.Temperature
	}

	resp, err := llmClient.CreateChatCompletion(ctx, req)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		c.log.Errorf("LLM API调用失败: %v", err)
		return "", errors.New("LLM服务暂不可用, 请稍后再试")
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", errors.New("LLM服务返回了空结果")
	}
	return c.filterContent(resp.Choices[0].Message.Content), nil
}

func (c *client) GenerateStandardQuestion(ctx context.Context, prompt enum.SystemPrompt, text string) (string, error) {
	return c.GetCompletion(ctx, enum.ModelSmall, prompt, text, 0.2)
}

func (c *client) GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error) {
	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	llmConfig := c.getLlmConfig(size)
	if llmConfig == nil || llmConfig.Model == "" {
		return "", errors.New("未找到指定的LLM客户端配置")
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: string(systemPrompt),
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: content,
		},
	}

	req := openai.ChatCompletionRequest{
		Model:    llmConfig.Model,
		Messages: messages,
	}

	// 优先使用传入的temperature参数，其次是配置文件中的，最后使用LLM默认值
	if len(temperature) > 0 {
		req.Temperature = temperature[0]
	} else if llmConfig.Temperature != nil {
		req.Temperature = *llmConfig.Temperature
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

	return c.filterContent(resp.Choices[0].Message.Content), nil
}
