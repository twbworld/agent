package enum

type DbType string

const (
	MYSQL  DbType = `mysql`
	SQLITE DbType = `sqlite3`
)

type Msg string

const (
	DefaultSuccessMsg Msg = `ok`
	DefaultFailMsg    Msg = `错误`
)

type ResCode int8

const (
	SuccessCode   ResCode = 0
	ErrorCode     ResCode = 1
	AuthErrorCode ResCode = 2
)

type SystemPrompt string

const (
	PromptNoThink SystemPrompt = `你是一个简洁、直接的AI助手。请直接提供最终答案，不要包含任何思考步骤、推理过程或解释。`
)

type LlmSize string

const (
	ModelSmall  LlmSize = "small"
	ModelMedium LlmSize = "medium"
	ModelLarge  LlmSize = "large"
)

type TransferToHuman string

const (
	TransferToHuman1 TransferToHuman = "用户要求[转人工]"
	TransferToHuman2 TransferToHuman = "系统错误导致[转人工]"
	TransferToHuman3 TransferToHuman = "自动[转人工]"
	TransferToHuman4 TransferToHuman = "用户情绪激动[转人工]"
	TransferToHuman5 TransferToHuman = "AI无法处理[转人工]"
	TransferToHuman6 TransferToHuman = "金额过大[转人工]"
)

// ChatwootWebhook defines constants for Chatwoot webhook payload values.
type ChatwootWebhook string

const (
	// MessageTypeIncoming represents a new message from the contact.
	MessageTypeIncoming ChatwootWebhook = "incoming"
	// MessageTypeOutgoing represents a message sent from the application.
	MessageTypeOutgoing ChatwootWebhook = "outgoing"
	// SenderTypeContact represents the end-user in the conversation.
	SenderTypeContact ChatwootWebhook = "contact"
)