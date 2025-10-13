package common

import "gitee.com/taoJie_1/chat/internal/chatwoot"

type KeywordsList struct {
	ShortCode string `db:"short_code" json:"short_code"`
	Content   string `db:"content" json:"content"`
}

// 代表一条完整的关键词规则，用于内部传递
type KeywordRule struct {
	chatwoot.CannedResponse
	Embedding []float32 // 文本向量
}
