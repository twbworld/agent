package common

type ChatRequestSender struct {
	Type string `json:"type"`
}

type ChatRequestConversation struct {
	AccountID      uint `json:"account_id"`
	ConversationID uint `json:"id"`
}

// ChatRequest is the main structure for incoming webhook requests from Chatwoot.
type ChatRequest struct {
	Content      string                  `json:"content"`
	Conversation ChatRequestConversation `json:"conversation"`
	MessageType  string                  `json:"message_type"`
	Sender       ChatRequestSender       `json:"sender"`
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}
