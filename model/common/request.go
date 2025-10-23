package common

import "gitee.com/taoJie_1/mall-agent/model/enum"

// ChatRequest 对应 Chatwoot webhook 发送过来的消息体
type ChatRequest struct {
	Event
	ID           uint         `json:"id"`
	Content      string       `json:"content"`
	MessageType  string       `json:"message_type"`
	CreatedAt    string       `json:"created_at"` // 时间戳
	Private      bool         `json:"private"`    // 是否是私密消息
	Conversation Conversation `json:"conversation"`
	Sender       Sender       `json:"sender"`
	Account      Account      `json:"account"`
	Attachments  []Attachment `json:"attachments"`
}

type ConversationResolvedRequest struct {
	Event
	ID uint `json:"id"`
}

type Event struct {
	Event enum.ChatwootEvent `json:"event"`
}

// Account 代表账户信息
type Account struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Conversation 代表会话信息
type Conversation struct {
	ConversationID uint   `json:"id"`
	AccountID      uint   `json:"account_id"` // 该字段将由 ChatRequest.Account.ID 手动填充
	InboxID        uint   `json:"inbox_id"`
	Status         string `json:"status"`
	Meta           Meta   `json:"meta"`
}

// Meta 存放会话的元数据
type Meta struct {
	Sender SenderInfo `json:"sender"`
}

// SenderInfo 代表元数据中的发送者信息
type SenderInfo struct {
	Type string `json:"type"` // "contact", "agent_bot", "user"
}

// Sender 代表消息的直接发送者信息
type Sender struct {
	ID    uint    `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Type  string  `json:"type"`
}

// Attachment 代表附件信息
type Attachment struct {
	ID        uint   `json:"id"`
	MessageID uint   `json:"message_id"`
	FileType  string `json:"file_type"` // "image", "audio", "video", "file"
	DataURL   string `json:"data_url"`
}
