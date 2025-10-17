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
	SystemPromptDefault                SystemPrompt = `你是一个AI商城客服助手，请直接回答用户的问题，不要包含任何额外的思考或解释过程。回答的语言应与用户的问题语言一致。`
	SystemPromptGenQuestionFromContent SystemPrompt = `你是一个逆向问题生成AI。请仔细阅读下面提供的“答案”文本，然后生成一个或多个最匹配该答案的、最自然的“用户问题”。
- 思考：想象一个以简体中文为母语的真实用户，他会问什么样的问题，才能得到这个答案？
- 风格：问题应该简短、口语化，就像在聊天窗口输入一样。
- 输出：只输出最终的中文问题，不要包含任何解释、标签或引号。`
	SystemPromptGenQuestionFromKeyword SystemPrompt = `你是一个专门优化用户查询的AI。请将用户提供的“关键词”或“种子问题”，转换成一个或多个真实用户提问习惯的“标准问题”。
- 风格：自然、口语化、直接。
- 目标：生成的问题将用于向量匹配，所以它必须精准地捕捉核心意图。
- 输出：只输出最终的中文问题，不要包含任何解释、标签或引号。`
	SystemPromptRAG SystemPrompt = `你是一个专业的AI商城客服。请根据下面以Q&A形式提供的“参考资料”来回答用户的“问题”。
- 如果参考资料与问题相关，请基于参考资料的内容，以自然、友好的语气进行回答。
- 如果参考资料与问题无关，请忽略参考资料，并像没有参考资料一样，直接回答用户的问题。
- 你的回答应该简洁、清晰，并且直接面向用户。
- 回答的语言应与用户的问题语言一致。
- 禁止在回答中提及“参考资料”。`
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
