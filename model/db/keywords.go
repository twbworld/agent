package db

import "gitee.com/taoJie_1/chat/pkg/chatwoot"

type Keywords struct {
	BaseField
	ShortCode string `db:"short_code" json:"short_code" info:"关键词"`
	Content   string `db:"content" json:"content" info:"内容"`
	AccountId uint   `db:"account_id" json:"account_id" info:"账号id"`
}

func (Keywords) TableName() string {
	return `keywords`
}

// 代表一条完整的关键词规则，用于内部传递
type KeywordRule struct {
	chatwoot.CannedResponse
	Embedding []float32 // 文本向量
}
