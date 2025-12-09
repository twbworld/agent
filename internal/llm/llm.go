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
	// 调用LLM进行实时对话，并支持传入历史消息
	ChatCompletionWithHistory(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, history []common.LlmMessage, temperature ...float32) (string, error)
	// 执行一次性的文本生成任务，通常用于后台任务。
	GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error)
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
	// 简单情况: 标签在末尾或分割 (原逻辑保留)
	startTag := "<think>"
	if parts := strings.SplitN(rawAnswer, startTag, 2); len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}

	endTag := "</think>"

	sIdx := strings.Index(rawAnswer, startTag)
	eIdx := strings.Index(rawAnswer, endTag)

	// 如果包含完整的闭合标签
	if sIdx != -1 && eIdx != -1 && eIdx > sIdx {
		// 删除 <think>... content ...</think>
		// 保留 sIdx 之前的内容 和 eIdx + len(endTag) 之后的内容
		before := rawAnswer[:sIdx]
		after := rawAnswer[eIdx+len(endTag):]
		return strings.TrimSpace(before + after)
	}

	return strings.TrimSpace(rawAnswer)
}

// createChatCompletion 是调用LLM API的统一内部方法
func (c *client) createChatCompletion(ctx context.Context, size enum.LlmSize, messages []openai.ChatCompletionMessage, temperature ...float32) (string, error) {
	llmClient, ok := c.llmClients[size]
	if !ok {
		return "", errors.New("未找到指定大小的LLM客户端实例")
	}
	llmConfig := c.getLlmConfig(size)
	if llmConfig == nil || llmConfig.Model == "" {
		return "", errors.New("未找到指定的LLM客户端配置")
	}

	req := openai.ChatCompletionRequest{
		Model:    llmConfig.Model,
		Messages: messages,
	}

	if len(temperature) > 0 {
		req.Temperature = temperature[0]
	} else if llmConfig.Temperature != nil {
		req.Temperature = *llmConfig.Temperature
	}

	resp, err := llmClient.CreateChatCompletion(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			return "", context.Canceled
		}
		c.log.Errorf("LLM API调用失败: %v", err)
		return "", errors.New("LLM服务暂不可用, 请稍后再试")
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", errors.New("LLM服务返回了空结果")
	}

	return c.filterContent(resp.Choices[0].Message.Content), nil
}

// systemPrompt: LLM的系统提示词
// content: 用户问题 + 知识库参考资料 (RAG)
// history: 之前的对话历史消息列表
func (c *client) ChatCompletionWithHistory(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, history []common.LlmMessage, temperature ...float32) (string, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: string(systemPrompt),
		},
	}

	for _, msg := range history {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if content != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: content,
		})
	}

	return c.createChatCompletion(ctx, size, messages, temperature...)
}

func (c *client) GetCompletion(ctx context.Context, size enum.LlmSize, systemPrompt enum.SystemPrompt, content string, temperature ...float32) (string, error) {
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
	return c.createChatCompletion(ctx, size, messages, temperature...)
}
