package llm

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"github.com/twbworld/agent/model/common"
	"github.com/twbworld/agent/model/config"
	"github.com/twbworld/agent/model/enum"
)

type CtxKey string

const DisableThinkingKey CtxKey = "disable_thinking"

// ChatRequest 统一封装对 LLM 调用的所有参数诉求
type ChatRequest struct {
	Size            enum.LlmSize
	SystemPrompt    enum.SystemPrompt
	Messages        []common.LlmMessage // 包含上下文历史以及当前最新问题/工具反馈
	Tools           []openai.Tool       // 可选：如果不为空，则启用原生 Function Calling 工具调用
	SchemaName      enum.SchemaName     // 可选：如果不为空，则启用 Structured Outputs(结构化输出)
	Schema          any                 // 对应的 JSON Schema 定义
	Strict          bool                // Structured Outputs 的严格模式开关
	Temperature     *float32            // 可选：如果为 nil，则回退读取配置中的默认值
	DisableThinking bool                //是否在底层API关闭LLM思考过程(thinking)
}

type Service interface {
	// Chat 所有的 LLM 调用需求（普通对话、工具调用、结构化输出）
	Chat(ctx context.Context, req *ChatRequest) (*openai.ChatCompletionMessage, error)
}

type client struct {
	llmClients map[enum.LlmSize]*openai.Client
	llmConfigs []config.Llm
}

// NewClient 创建一个新的LLM客户端实例，并通过依赖注入初始化
func NewClient(log *logrus.Logger, clients map[enum.LlmSize]*openai.Client, configs []config.Llm) Service {
	return &client{
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
	if len(c.llmConfigs) > 0 {
		return &c.llmConfigs[0]
	}
	return nil
}

// filterContent 从LLM的原始响应中剥离思考过程标签
func (c *client) filterContent(rawAnswer string) string {
	startTag := "<think>"
	if parts := strings.SplitN(rawAnswer, startTag, 2); len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}

	endTag := "</think>"
	sIdx := strings.Index(rawAnswer, startTag)
	eIdx := strings.Index(rawAnswer, endTag)

	if sIdx != -1 && eIdx != -1 && eIdx > sIdx {
		before := rawAnswer[:sIdx]
		after := rawAnswer[eIdx+len(endTag):]
		return strings.TrimSpace(before + after)
	}

	return strings.TrimSpace(rawAnswer)
}

func (c *client) Chat(ctx context.Context, req *ChatRequest) (*openai.ChatCompletionMessage, error) {
	llmConfig := c.getLlmConfig(req.Size)
	if llmConfig == nil || llmConfig.Model == "" {
		return nil, errors.New("未找到指定的LLM客户端配置")
	}
	if req.DisableThinking {
		ctx = context.WithValue(ctx, DisableThinkingKey, true)
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages)+1)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: string(req.SystemPrompt),
	})

	for _, msg := range req.Messages {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		})
	}

	openaiReq := openai.ChatCompletionRequest{
		Model:    llmConfig.Model,
		Messages: messages,
	}

	// 1. 按需装载 Function Calling
	if len(req.Tools) > 0 {
		openaiReq.Tools = req.Tools
	}

	// 2. 按需装载 Structured Outputs
	if req.SchemaName != "" && req.Schema != nil {
		var schemaMarshaler json.Marshaler
		switch s := req.Schema.(type) {
		case json.Marshaler:
			schemaMarshaler = s
		case []byte:
			schemaMarshaler = jsontext.Value(s)
		case string:
			schemaMarshaler = jsontext.Value(s)
		default:
			schemaBytes, err := json.Marshal(req.Schema)
			if err != nil {
				return nil, fmt.Errorf("序列化 JSON Schema 失败: %w", err)
			}
			schemaMarshaler = jsontext.Value(schemaBytes)
		}

		openaiReq.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   string(req.SchemaName),
				Schema: schemaMarshaler,
				Strict: req.Strict,
			},
		}
	}

	// 3. 设定采样温度
	if temp := cmp.Or(req.Temperature, llmConfig.Temperature); temp != nil {
		openaiReq.Temperature = *temp
	}

	llmClient, ok := c.llmClients[req.Size]
	if !ok {
		return nil, errors.New("未找到指定大小的LLM客户端实例")
	}

	resp, err := llmClient.CreateChatCompletion(ctx, openaiReq)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}
		return nil, fmt.Errorf("LLM API调用失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("LLM服务返回了空结果")
	}

	msg := resp.Choices[0].Message
	// 纯工具调用时 Content 为空
	if msg.Content != "" {
		// 有些LLM平台在Content内用<think>标签装思维链, 去除
		// 有些平台如DeepSeek/百炼则支持单独属性装思维链, 用msg.ReasoningContent取
		msg.Content = c.filterContent(msg.Content)
	}

	return &msg, nil
}
