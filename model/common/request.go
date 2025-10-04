package common

type ChatRequestSender struct {
	Type string `json:"type"`
}

type ChatRequestConversation struct {
	AccountID      uint `json:"account_id"`
	ConversationID uint `json:"id"`
}

type ChatRequest struct {
	Content      string                  `json:"content"`
	Conversation ChatRequestConversation `json:"conversation"`
	MessageType  string                  `json:"message_type"`
	Sender       ChatRequestSender       `json:"sender"`
}
