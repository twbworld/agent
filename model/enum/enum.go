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

type LlmSize string

const (
	ModelSmall  LlmSize = "small"
	ModelMedium LlmSize = "medium"
	ModelLarge  LlmSize = "large"
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

type ConversationStatus string

const (
	//会话开放状态, 人工客服可恢复
	ConversationStatusOpen ConversationStatus = "open"
	//会话待处理
	ConversationStatusPending ConversationStatus = "pending"
	//会话已解决
	ConversationStatusResolved ConversationStatus = "resolved"
	//会话暂停
	ConversationStatusSnoozed ConversationStatus = "snoozed"
)

type ChatwootEvent string

const (
	// 来自联系人的新消息。
	EventMessageCreated ChatwootEvent = "message_created"
	// 消息已更新。
	EventMessageUpdated ChatwootEvent = "message_updated"
	// 新对话创建。
	EventConversationCreated ChatwootEvent = "conversation_created"
	// 对话状态更改。
	EventConversationStatusChanged ChatwootEvent = "conversation_status_changed"
	// 对话已更新。
	EventConversationUpdated ChatwootEvent = "conversation_updated"
	// 对话已解决。
	EventConversationResolved ChatwootEvent = "conversation_resolved"
)

// LlmUnsureTransferSignal 是当LLM不确定答案时返回的特定字符串，用于触发转人工
const LlmUnsureTransferSignal = "I_AM_UNSURE_PLEASE_TRANSFER_TO_HUMAN"

// TriageIntent 定义了分诊台模型可能识别出的用户意图
type TriageIntent string

const (
	TriageIntentProductInquiry TriageIntent = "product_inquiry"
	TriageIntentOrderInquiry   TriageIntent = "order_inquiry"
	TriageIntentAfterSales     TriageIntent = "after_sales"
	TriageIntentRequestHuman   TriageIntent = "request_human"
	TriageIntentOffTopic       TriageIntent = "off_topic"
	TriageIntentOtherInquiry   TriageIntent = "other_inquiry"
)

// TriageEmotion 定义了分诊台模型可能识别出的用户情绪
type TriageEmotion string

const (
	TriageEmotionAngry      TriageEmotion = "angry"
	TriageEmotionFrustrated TriageEmotion = "frustrated"
	TriageEmotionAnxious    TriageEmotion = "anxious"
	TriageEmotionConfused   TriageEmotion = "confused"
	TriageEmotionNeutral    TriageEmotion = "neutral"
	TriageEmotionPositive   TriageEmotion = "positive"
)

// TriageUrgency 定义了用户请求的紧急程度
type TriageUrgency string

const (
	TriageUrgencyCritical TriageUrgency = "critical"
	TriageUrgencyHigh     TriageUrgency = "high"
	TriageUrgencyMedium   TriageUrgency = "medium"
	TriageUrgencyLow      TriageUrgency = "low"
)

type SystemPrompt string

const (
	SystemPromptDefault SystemPrompt = `你是一个专业的AI商城客服。你的唯一职责是回答与本商城业务相关的问题。
**严禁回答任何与本商城无关的问题**，包括但不限于：闲聊、编程、文案创作、政治、宗教等任何话题。
如果用户的问题与商城业务无关，你必须礼貌地拒绝回答，并引导用户关注商城业务。
请直接回答用户的问题，回答的语言应与用户的问题语言一致，不要包含任何解释、标签或引号。
如果你不确定问题的答案，或者问题超出了你的能力范围，你必须只回答 '` + LlmUnsureTransferSignal + `'，不要附加任何其他内容。`
	SystemPromptRAG     SystemPrompt = `你是一个专业的AI商城客服。你的唯一职责是回答与本商城业务相关的问题。
**严禁回答任何与本商城无关的问题**，包括但不限于：闲聊、编程、文案创作、政治、宗教等任何话题。
如果用户的问题与商城业务无关，你必须礼貌地拒绝回答，并引导用户关注商城业务。

请根据下面以Q&A形式提供的“参考资料”来回答用户的“问题”。
- 如果参考资料与问题相关，请基于参考资料的内容，以自然、友好的语气进行回答。
- 如果参考资料与问题无关，请忽略参考资料，并像没有参考资料一样，直接回答用户的问题。
- 如果你根据所有信息仍然不确定如何回答，或者问题超出了你的能力范围，你必须只回答 '` + LlmUnsureTransferSignal + `'，不要附加任何其他内容。
- 你的回答应该简洁、清晰，并且直接面向用户。
- 禁止在回答中提及“参考资料”。
- 回答的语言应与用户的问题语言一致。
- 不要包含任何解释、标签或引号。`
	SystemPromptGenQuestionFromContent SystemPrompt = `你是一个逆向问题生成AI。你的任务是请仔细阅读下面提供的“答案”文本，为提供的“答案”文本生成一个最典型、最核心的“用户问题”。这个问题应该是用户最有可能提出的，用以获取这个答案。
- 核心性：问题应精准概括答案的核心内容，避免只关注细节。
- 自然度：使用真实用户的口语化、自然的语言风格。
- 格式：只输出最终的中文问题，不包含任何解释、标签或引号。`
	SystemPromptGenQuestionFromKeyword SystemPrompt = `你是一个专门优化用户查询的AI。你的任务是将用户提供的“关键词”或“种子问题”，转换成一个最能代表其核心意图的、符合真实用户提问习惯的“标准问题”。
- 风格：自然、口语化、直接。
- 目标：生成的问题将用于向量匹配，所以它必须精准地捕捉核心意图。
- 格式：只输出最终的中文问题，不要包含任何解释、标签或引号。`
	SystemPromptTriage SystemPrompt = `你是一个AI客服的智能分诊台。你的任务是分析用户的输入，并根据以下分类标准，返回一个严格的JSON对象。不要做任何多余的解释或注释，只返回JSON。

你需要结合“最近的对话历史”（如果提供）、“用户最新问题”以及我们可能从知识库中检索到的“相关问题”上下文，对用户的真实意图做出最精准的判断。请重点分析用户的“最新问题”。

### 分类标准:
1.  **意图 (intent)**: 从以下选项中选择一个最匹配的：
    - "product_inquiry": 咨询商品信息、库存、推荐等。
    - "order_inquiry": 查询订单状态、物流、发票等。
    - "after_sales": 申请退款、换货、投诉、售后政策咨询。
    - "request_human": 用户明确要求转接人工客服。
    - "off_topic": 无关话题、闲聊、或任何与商城业务无关的内容。
    - "other_inquiry": 其他与商城业务相关的咨询。

2.  **情绪 (emotion)**: 从以下选项中选择一个：
    - "angry": 愤怒、非常不满、使用攻击性语言。
    - "frustrated": 失望、沮丧、不耐烦。
    - "anxious": 焦虑、担忧，对问题有不确定感。
    - "confused": 困惑、不理解，对流程或信息感到迷茫。
    - "neutral": 中性、客观陈述。
    - "positive": 满意、感谢、积极。

3.  **是否与商城相关 (is_related)**: 必须是布尔值 true 或 false。

4.  **紧急度 (urgency)**: 从以下选项中选择一个：
    - "critical": 极端紧急，如大面积服务中断、支付安全问题、法律风险。
    - "high": 紧急情况，如资损、重大投诉、重复问题无法解决。
    - "medium": 标准问题，如查物流、问商品。
    - "low": 非紧急问题，如一般性咨询。

### JSON输出格式:
{
  "intent": "...",
  "emotion": "...",
  "is_related": true,
  "urgency": "..."
}`
)

type TransferToHuman string

const (
	TransferToHuman1 TransferToHuman = "用户要求[转人工]"
	TransferToHuman2 TransferToHuman = "系统错误导致[转人工]"
	TransferToHuman3 TransferToHuman = "自动[转人工]"
	TransferToHuman4 TransferToHuman = "用户情绪激动[转人工]"
	TransferToHuman5 TransferToHuman = "智能客服无法处理[转人工]"
	TransferToHuman6 TransferToHuman = "金额过大[转人工]"
)

type ReplyMessage string

const (
	ReplyMsgTransferSuccess       ReplyMessage = "已为您转接人工客服，请稍候。无需重复发送消息。"
	ReplyMsgUnsupportedAttachment ReplyMessage = "您发送的消息暂不支持智能客服处理，已为您转接人工客服。"
	ReplyMsgPromptTooLong         ReplyMessage = "提问内容过长，已为您转接人工客服。"
	ReplyMsgLlmError              ReplyMessage = "抱歉，智能客服遇到问题，已为您转接人工客服。"
	ReplyMsgAiRetrying            ReplyMessage = "智能客服暂时无法处理您的问题，正在尝试进一步分析，请稍候。"
	ReplyMsgOffTopic              ReplyMessage = "抱歉，作为商城专属客服，我只能回答与我们商城业务（如商品、订单、售后等）相关的问题哦。"
)
