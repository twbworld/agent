package chatwoot

type CannedResponse struct {
	Id        int    `json:"id"`
	AccountId int    `json:"account_id"`
	ShortCode string `json:"short_code"`
	Content   string `json:"content"`
}

type Service interface {
	GetCannedResponses() ([]CannedResponse, error)
	CreatePrivateNote(conversationID uint, content string) error
	ToggleConversationStatus(conversationID uint) error
	ToggleTypingStatus(conversationID uint, status string) error
	CreateMessage(conversationID uint, content string) error
}
