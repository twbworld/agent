package common

// ChatRequest 对应 Chatwoot webhook 发送过来的消息体
type ChatRequest struct {
	ID           uint         `json:"id"`
	Content      string       `json:"content"`
	MessageType  string       `json:"message_type"`
	CreatedAt    int64        `json:"created_at"`
	Conversation Conversation `json:"conversation"`
	Sender       Sender       `json:"sender"`
	Attachments  []Attachment `json:"attachments"`
}

// Conversation 代表会话信息
type Conversation struct {
	ConversationID uint `json:"id"`
	AccountID      uint `json:"account_id"`
	InboxID        uint `json:"inbox_id"`
}

// Sender 代表发送者信息
type Sender struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Type  string `json:"type"` // "contact", "agent_bot", "user"
}

// Attachment 代表附件信息
type Attachment struct {
	ID        uint   `json:"id"`
	MessageID uint   `json:"message_id"`
	FileType  string `json:"file_type"` // "image", "audio", "video", "file"
	DataURL   string `json:"data_url"`
}
