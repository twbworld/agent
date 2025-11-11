package common

import (
	"encoding/json"

	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
)

type KeywordsList struct {
	ShortCode string `db:"short_code" json:"short_code"`
	Content   string `db:"content" json:"content"`
}

// 代表一条完整的关键词规则，用于内部传递
type KeywordRule struct {
	chatwoot.CannedResponse
	Embedding []float32 // 文本向量
}

// LlmMessage 结构体定义了发送给LLM的聊天消息格式
type LlmMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user", "assistant", "system"
	Content string `json:"content"` // 消息内容
}

// TriageResult 结构体定义了分诊台LLM返回的JSON格式
// 注意：此结构体的定义必须与 model/enum/enum.go 中的 SystemPromptTriage 提示词所描述的JSON格式保持同步。
type TriageResult struct {
	Intent  string `json:"intent"`
	Emotion string `json:"emotion"`
	Urgency string `json:"urgency"`
}

// ToolCallParams 定义了LLM返回的工具调用JSON的结构。
// 注意：此结构体的定义必须与 model/enum/enum.go 中的 SystemPromptToolUser 提示词所描述的JSON格式保持同步。
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ReloadPost struct {
	Name string `json:"name"`
}
