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

// KeywordType 定义了知识库中问题的匹配类型
type KeywordType string

const (
	// KeywordTypeSemantic 仅用于AI语义匹配
	KeywordTypeSemantic KeywordType = "AI_SEMANTIC"
	// KeywordTypeHybrid 用于精确匹配和AI语义匹配
	KeywordTypeHybrid KeywordType = "HYBRID"
	// KeywordTypeExact 仅用于精确匹配
	KeywordTypeExact KeywordType = "EXACT"
)

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

// LlmUnsureTransferSignal 是当LLM不确定答案时返回的特定字符串，用于触发转人工
const LlmUnsureTransferSignal = "I_AM_UNSURE_PLEASE_TRANSFER_TO_HUMAN"

type SystemPrompt string

const (
	SystemPromptGenQuestionFromContent SystemPrompt = `任务：阅读“答案”文本，生成一个最核心、最典型的“用户问题”以获取该答案。
要求：
1. 精准概括核心内容，避免只关注细节。
2. 使用自然口语化的中文。
3. 仅输出问题本身，禁止出现解释标签。`

	SystemPromptGenQuestionFromKeyword SystemPrompt = `任务：将“关键词”或“种子问题”转换为最能代表核心意图的“标准问题”。
要求：
1. 风格自然、口语化、直接。
2. 精准捕捉意图以用于向量匹配。
3. 仅输出最终中文问题，禁止出现解释标签。`

	SystemPromptGenQuestionInstruction SystemPrompt = `生成3个表述不同的相关用户问题。每行一个，禁止出现解释标签或序号。`

	SystemPromptTriage SystemPrompt = `你是商城AI客服的智能分诊台。分析用户输入，返回严格JSON。结合“最近的对话历史”（如果提供）、根据“用户最新问题”，在知识库中检索到的“可能相关的问题”（如果提供）以及“用户最新问题”，判断“用户最新问题”的意图。

### 字段说明
1. **answer_id**: 若“可能相关的问题”列表第N个与用户的核心意图上是一致的，填对应的编号 N；否则 0。
2. **intent**:
    - product_inquiry: 咨询商品信息、库存、推荐等。
    - order_inquiry: 查询订单状态、物流、发票等。
    - after_sales: 申请退款、换货、投诉、售后政策咨询。
    - request_human: 用户明确要求转接人工客服。
    - off_topic: 无关话题、闲聊、或任何与商城业务无关的内容。
    - other_inquiry: 其他与商城业务或知识库相关的咨询、问候语、或无法归类但非无关的话题。
   *注: 若answer_id>0，优先选"product_inquiry" 或 "other_inquiry"。
3. **emotion**:
    - angry: 愤怒、非常不满、使用攻击性语言。
    - frustrated: 失望、沮丧、不耐烦。
    - anxious: 焦虑、担忧，对问题有不确定感。
    - confused: 困惑、不理解，对流程或信息感到迷茫。
    - neutral: 中性、客观陈述。
    - positive: 满意、感谢、积极。
4. **urgency**:
    - critical: 极端紧急，如大面积服务中断、支付安全问题、法律风险。
    - high: 紧急情况，涉及高额资损(如金额>1000元争议)、重大投诉、重复问题无法解决。
    - medium: 标准问题，如查物流、问商品。
    - low: 非紧急问题，如一般性咨询。

### JSON格式
{
  "intent": "...",
  "emotion": "...",
  "urgency": "...",
  "answer_id": 0
}`

	SystemPromptDefault SystemPrompt = `你是专业商城AI客服，只回答业务相关问题。
1. 严禁闲聊或回答无关话题（如编程、政治等），遇此必须礼貌拒绝并引导回业务。
2. 回答内容必须简洁、自然、与提问语言一致，禁止出现解释标签。
4. 若无法回答或不确定，仅输出 '` + LlmUnsureTransferSignal + `'，不加任何内容。`

	SystemPromptRAG SystemPrompt = `你是专业的商城AI客服，只回答业务相关问题。
依据“参考资料”回答“用户最新问题”：
1. 严禁闲聊或回答无关话题（如编程、政治等），遇此必须礼貌拒绝并引导回业务。
2. 若资料相关，用自然语气回答；若无关，忽略资料直接回答。
3. 回答需简洁清晰，直接面向用户，**严禁提及“参考资料”**。
4. 语言与提问一致，禁止出现解释标签。
5. 若无法回答或不确定，仅输出 '` + LlmUnsureTransferSignal + `'，不加任何内容。`

	SystemPromptToolUser SystemPrompt = `你可调用外部工具来完成任务。
若需工具，必须使用 <tool_code>...</tool_code> 包裹JSON数组：[{"name": "mall.query_goods", "arguments": {...}}]。

**核心原则**:
1.  **优先使用上下文和对话历史记录**: 在决定调用工具之前，请务必仔细检查“上下文信息”和“对话历史”。
    - 如果“上下文信息”或“对话历史”中包含了答案，且属于非实时性信息(如商品名称、订单价格),用户也没有明确要求“刷新”或查询“最新”数据，请**直接回答**。
	- 如果**直接回答**, 回答内容必须简洁、自然、与提问语言一致，禁止出现解释标签。
    - **严禁**在已有答案的情况下重复调用工具。
2. **参数完整性**: 如果调用工具所需的参数缺失，你必须向用户提问以获取缺失的信息，而不是直接放弃或猜测。
3. **工具命名**: 工具的 "name" 必须使用 "客户端.工具名" 格式, 工具的 "arguments"是一个包含所有参数键值对的对象。

--- 可用工具 ---
{tools}

请根据“用户最新问题”、“上下文信息”和“可用工具”列表，决定是直接回答、向用户提问，还是生成工具调用JSON。`

	SystemPromptSynthesizeToolResult SystemPrompt = `你是商城AI客服。根据“工具执行结果”和“对话历史”，回答用户。
1. 基于这些工具执行的结果，结合“对话历史”，像查阅后台的真人客服一样回答，**禁止提及"工具"或"MCP"或"tool"**。
2. 如果工具返回大量数据（如多个订单），请只总结最近的一条或最相关的一条，除非用户要求列出所有。
3. 若工具报错或无数据，礼貌回答。
4. 回答内容必须简洁、自然、与提问语言一致，禁止出现解释标签。
5. 若根据所有信息仍无法回答，仅输出 '` + LlmUnsureTransferSignal + `'，禁止附加任何内容。`
)

type TransferToHuman string

const (
	TransferToHuman1 TransferToHuman = "用户要求[转人工]"
	TransferToHuman2 TransferToHuman = "Agent系统错误导致[转人工]"
	TransferToHuman3 TransferToHuman = "自动[转人工]"
	TransferToHuman4 TransferToHuman = "用户情绪激动[转人工]"
	TransferToHuman5 TransferToHuman = "智能客服无法处理[转人工]"
	TransferToHuman6 TransferToHuman = "高风险业务/金额过大[转人工]"
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
