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
	SystemPromptDefault           SystemPrompt = "你是一个AI商城客服助手，请直接回答用户的问题，不要包含任何额外的思考或解释过程。"
	SystemPromptForCannedResponse SystemPrompt = "你是一个世界级的语言学家和知识库专家。你的任务是根据下面提供的'回答'内容，生成一个最能概括其核心思想的、简洁的'标准问题'。'标准问题'应该像是用户会直接提出的问题。只返回问题本身，不要包含任何额外的前缀、引号或解释，不要包含任何额外的思考或解释过程。"
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

type ChatwootWebhook string

const (
	// 来自联系人的新消息。
	MessageTypeIncoming ChatwootWebhook = "incoming"
	// 从应用程序发送的消息。
	MessageTypeOutgoing ChatwootWebhook = "outgoing"
	// 对话中的最终用户。
	SenderTypeContact ChatwootWebhook = "contact"
)
