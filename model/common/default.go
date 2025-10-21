package common

import "gitee.com/taoJie_1/mall-agent/internal/chatwoot"

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
