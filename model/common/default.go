package common

import (
	"encoding/json"
)

// LlmMessage 结构体定义了发送给LLM的聊天消息格式
type LlmMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user", "assistant", "system"
	Content string `json:"content"` // 消息内容
}

// TriageResult 结构体定义了分诊台LLM返回的JSON格式
// 注意：此结构体的定义必须与 model/enum/enum.go 中的 SystemPromptTriage 提示词所描述的JSON格式保持同步。
type TriageResult struct {
	Intent   string `json:"intent"`
	Emotion  string `json:"emotion"`
	Urgency  string `json:"urgency"`
	// 匹配到的参考问题编号 (0表示无匹配)
	AnswerID int    `json:"answer_id"`
}

// ToolCallParams 定义了LLM返回的工具调用JSON的结构。
// 注意：此结构体的定义必须与 model/enum/enum.go 中的 SystemPromptToolUser 提示词所描述的JSON格式保持同步。
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCalls 定义了一个ToolCallParams的切片，用于表示多个工具调用
type ToolCalls []ToolCallParams
