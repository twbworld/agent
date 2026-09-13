package common

import (
	"github.com/sashabaranov/go-openai"
	"github.com/twbworld/agent/model/enum"
)

// LlmMessage 结构体定义了发送给LLM的聊天消息格式
type LlmMessage struct {
	Role       string            `json:"role"`                   // 消息角色，例如 "user", "assistant", "system", "tool"
	Content    string            `json:"content"`                // 消息内容
	ToolCalls  []openai.ToolCall `json:"tool_calls,omitempty"`   // Assistant角色时，原生的工具调用列表
	ToolCallID string            `json:"tool_call_id,omitempty"` // Tool角色时，对应的调用ID
	Name       string            `json:"name,omitempty"`         // 对应的函数名称（Tool 角色使用）
}

// TriageResult 结构体定义了分诊台LLM返回的JSON格式
type TriageResult struct {
	Intent  string `json:"intent"`
	Emotion string `json:"emotion"`
	Urgency string `json:"urgency"`
	// 匹配到的参考问题编号 (0表示无匹配)
	AnswerID int `json:"answer_id"`
}

// LlmComplexResponse 定义了大型LLM复杂生成的最终结构化输出格式
type LlmComplexResponse struct {
	TransferToHuman bool                 `json:"transfer_to_human"`         // 是否需要转人工
	TransferReason  enum.TransferToHuman `json:"transfer_reason,omitempty"` // 具体的转人工原因枚举
	Reply           string               `json:"reply"`                     // 给用户的最终回复文本
}
