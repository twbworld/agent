package dto

// Question 代表与知识库条目关联的单个关键字/问题
type Question struct {
	ID        int    `json:"id"`       // 原始 Chatwoot 预设回复 ID
	Question  string `json:"question" binding:"required"` // short_code 的内容
	Type      string `json:"type"`     // '类型: 'AI_SEMANTIC', 'HYBRID', 'EXACT'
}

// KnowledgeItem 代表知识库UI中的单个条目，它将一个答案与多个问题分组。
type KnowledgeItem struct {
	ID        string      `json:"id"`        // 分组的唯一标识符 (例如，答案的哈希值)
	Answer    string      `json:"answer"`    // 预设回复的内容
	Questions []*Question `json:"questions"` // 关联的问题/关键字列表
	UpdatedAt int64       `json:"updatedAt,omitempty"` // 该知识条目下所有问题中最新的更新时间
}

// UpsertKnowledgeItemRequest 是创建或更新知识条目的请求体。
type UpsertKnowledgeItemRequest struct {
	// ID用于更新现有条目（内容哈希），新增时可为空。
	ID        string      `json:"id"`
	Answer    string      `json:"answer" binding:"required"`
	Questions []*Question `json:"questions" binding:"required,min=1,dive"`
}

// GenerateQuestionRequest 是 AI 问题生成助手的请求体。
type GenerateQuestionRequest struct {
	Context string `json:"context" binding:"required"`
	Type    string `json:"type"` // 'content' 或 'keyword'
}

// GenerateQuestionResponse 是 AI 问题生成助手的响应体。
type GenerateQuestionResponse struct {
	Questions []string `json:"questions"`
}
