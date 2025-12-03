package common

import (
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
)

type ReloadPost struct {
	Name string `json:"name"`
}

// 对应 Chatwoot webhook 的事件 'message_created' 消息体
type ChatRequest struct {
	Event
	ID           uint                     `json:"id"`
	Content      string                   `json:"content"`
	MessageType  chatwoot.ChatwootWebhook `json:"message_type"`
	CreatedAt    string                   `json:"created_at"` // 时间戳
	Private      bool                     `json:"private"`    // 是否是私密消息
	Conversation Conversation             `json:"conversation"`
	Sender       Sender                   `json:"sender"`
	Account      Account                  `json:"account"`
	Attachments  []Attachment             `json:"attachments"`
}

// 对应 Chatwoot webhook 的事件 'conversation_created' 消息体
type ConversationCreatedRequest struct {
	Event
	ID       uint                  `json:"id"`
	Meta     Meta                  `json:"meta"`
	Messages []MessageFromCreation `json:"messages"`
}

// 对应 Chatwoot webhook 的事件 'webwidget_triggered' 消息体
type WebwidgetTriggeredRequest struct {
	Contact struct {
		ID               uint             `json:"id"`
		CustomAttributes CustomAttributes `json:"custom_attributes"`
	} `json:"contact"`
}

// 对应 Chatwoot webhook 的事件 'conversation_resolved' 消息体
type ConversationResolvedRequest struct {
	Event
	ID uint `json:"id"`
}

type Event struct {
	Event chatwoot.ChatwootEvent `json:"event"`
}

type MessageFromCreation struct {
	ID          uint   `json:"id"`
	Content     string `json:"content"`
	AccountID   uint   `json:"account_id"`
	MessageType int    `json:"message_type"`
	Sender      Sender `json:"sender"`
}

// Attachment 代表附件信息
type Attachment struct {
	ID        uint   `json:"id"`
	MessageID uint   `json:"message_id"`
	FileType  string `json:"file_type"` // "image", "audio", "video", "file"
	DataURL   string `json:"data_url"`
}

// Conversation 代表会话信息
type Conversation struct {
	ID     uint                        `json:"id"`
	Status chatwoot.ConversationStatus `json:"status"`
	Meta   Meta                        `json:"meta"`
}

// Meta 存放会话的元数据
type Meta struct {
	Sender Sender `json:"sender"`
}

// Sender 代表元数据中的发送者信息
type Sender struct {
	ID               uint                `json:"id"`
	Type             chatwoot.SenderType `json:"type"`
	CustomAttributes CustomAttributes    `json:"custom_attributes"`
	Identifier       *string             `json:"identifier"`
	Name             *string             `json:"name"`
	PhoneNumber      *string             `json:"phone_number"`
	Email            *string             `json:"email"`
}

// Account 代表账户信息
type Account struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// 前端传的自定义属性
type CustomAttributes struct {
	GoodsID    string `json:"goods_id,omitempty"`
	GoodsTitle string `json:"goods_title,omitempty"`
	GoodsImage string `json:"goods_image,omitempty"`
	GoodsPrice string `json:"goods_price,omitempty"`
	GoodsUrl   string `json:"goods_url,omitempty"`

	OrderID string `json:"order_id,omitempty"`
}

// 定义了仪表板详情请求的JSON结构
type DashboardDetailsRequest struct {
	ContactID string `json:"contact_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	GoodsID   string `json:"goods_id,omitempty"`
	OrderID   string `json:"order_id,omitempty"`
}
