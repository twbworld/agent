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

type TriageDesc string

const (
	TriageDescIntent   TriageDesc = "用户意图分类。product_inquiry:咨询商品/库存等; order_inquiry:查订单/物流等; after_sales:退换货/投诉等售后; request_human:明确要求转人工; off_topic:闲聊/无关话题; other_inquiry:其他业务咨询。注:若answer_id>0，优先选 product_inquiry 或 other_inquiry。"
	TriageDescEmotion  TriageDesc = "用户情绪状态。angry:愤怒/攻击性; frustrated:失望/不耐烦; anxious:焦虑/担忧; confused:困惑/迷茫; neutral:中性客观; positive:满意/积极感谢。"
	TriageDescUrgency  TriageDesc = "请求紧急程度。critical:极端紧急(如大面积中断/资金安全); high:紧急(如高额资损/重大投诉); medium:标准(如查物流/问商品); low:非紧急(一般咨询)。"
	TriageDescAnswerID TriageDesc = "若提供的'可能相关的问题'列表中，有与用户的核心意图完全一致的项，填写其对应的编号N；若无匹配项或不确定，必须填0。"
)

type SystemPrompt string

const (
	SystemPromptGenQuestionFromContent SystemPrompt = `任务：阅读用户给的“答案”文本，生成3个表述不同的、最核心、最典型的“问题”以获取该答案。
要求：
1. 精准概括核心内容，避免只关注细节。
2. 使用自然口语化的中文。`

	SystemPromptGenQuestionFromKeyword SystemPrompt = `任务：阅读用户给的“关键词”或“种子问题”，生成3个最能代表核心意图的“标准问题”。
要求：
1. 风格自然、口语化、直接。
2. 精准捕捉意图以用于向量匹配。`

	SystemPromptTriage SystemPrompt = `你是商城AI客服的智能分诊台。请分析用户输入，结合“上下文信息”与“可能相关的问题”（如果有），精准判断用户的真实意图、情绪及紧急程度，并严格按照要求的 JSON 格式输出。`

	SystemPromptAgent SystemPrompt = `你是专业的商城AI客服，只回答业务相关问题。
1. 严禁闲聊或回答无关话题（如编程、政治等），遇此必须礼貌拒绝并引导回业务。
2. 若提供了“参考资料”，请依据资料回答；若资料相关，用自然语气回答；若无关，忽略资料直接回答。
3. 回答需简洁清晰，直接面向用户，**严禁提及“参考资料”等字眼**。
4. 语言必须与提问一致，禁止出现解释标签。
5. 若无法回答或不确定，请调用 transfer_to_human 工具。`
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

type SchemaName string

const (
	SchemaNameTriage           SchemaName = "TriageResult"
	SchemaNameGenerateQuestion SchemaName = "GenerateQuestionResponse"
)

// 虚拟调度工具名称
type Tool string

const (
	ToolTransferToHuman Tool = "transfer_to_human"
)
