package config

type Auth struct {
	Dashboard string `mapstructure:"dashboard" json:"dashboard" yaml:"dashboard"`
	Keyword   string `mapstructure:"keyword" json:"keyword" yaml:"keyword"`
}

type Mcp struct {
	Url  string `mapstructure:"url" json:"url" yaml:"url"`
	Auth string `mapstructure:"auth" json:"auth" yaml:"auth"`
}

type Database struct {
	Type          string `mapstructure:"type" json:"type" yaml:"type"`
	SqlitePath    string `mapstructure:"sqlite_path" json:"sqlite_path" yaml:"sqlite_path"`
	MysqlHost     string `mapstructure:"mysql_host" json:"mysql_host" yaml:"mysql_host"`
	MysqlPort     string `mapstructure:"mysql_port" json:"mysql_port" yaml:"mysql_port"`
	MysqlDbname   string `mapstructure:"mysql_dbname" json:"mysql_dbname" yaml:"mysql_dbname"`
	MysqlUsername string `mapstructure:"mysql_username" json:"mysql_username" yaml:"mysql_username"`
	MysqlPassword string `mapstructure:"mysql_password" json:"mysql_password" yaml:"mysql_password"`
}

type Redis struct {
	Addr                   string `mapstructure:"addr" json:"addr" yaml:"addr"`
	Password               string `mapstructure:"password" json:"password" yaml:"password"`
	DB                     int64  `mapstructure:"db" json:"db" yaml:"db"`
	LockExpiry             int64  `mapstructure:"lock_expiry" json:"lock_expiry" yaml:"lock_expiry"`
	ConversationHistoryTTL int64  `mapstructure:"conversation_history_ttl" json:"conversation_history_ttl" yaml:"conversation_history_ttl"`
	HistoryLockExpiry      int64  `mapstructure:"history_lock_expiry" json:"history_lock_expiry" yaml:"history_lock_expiry"`
}

type Teams struct {
	PreSalesID   int `mapstructure:"pre_sales_id" json:"pre_sales_id" yaml:"pre_sales_id"`
	AfterSalesID int `mapstructure:"after_sales_id" json:"after_sales_id" yaml:"after_sales_id"`
	DefaultID    int `mapstructure:"default_id" json:"default_id" yaml:"default_id"`
}

type Chatwoot struct {
	Url       string `mapstructure:"url" json:"url" yaml:"url"`
	AccountId int64  `mapstructure:"account_id" json:"account_id" yaml:"account_id"`
	Auth      string `mapstructure:"auth" json:"auth" yaml:"auth"`
	BotAuth   string `mapstructure:"bot_auth" json:"bot_auth" yaml:"bot_auth"`
	Teams     Teams  `mapstructure:"teams" json:"teams" yaml:"teams"`
}

type modelConfig struct {
	Url     string `mapstructure:"url" json:"url" yaml:"url"`
	Model   string `mapstructure:"model" json:"model" yaml:"model"`
	Auth    string `mapstructure:"auth" json:"auth" yaml:"auth"`
	Timeout int64  `mapstructure:"timeout" json:"timeout" yaml:"timeout"`
}

type Llm struct {
	modelConfig `mapstructure:",squash"`
	Size        string   `mapstructure:"size" json:"size" yaml:"size"`
	Temperature *float32 `mapstructure:"temperature" json:"temperature,omitempty" yaml:"temperature,omitempty"`
}

type LlmEmbedding struct {
	modelConfig  `mapstructure:",squash"`
	BatchTimeout int64 `mapstructure:"batch_timeout" json:"batch_timeout" yaml:"batch_timeout"`
}

type VectorDb struct {
	Url            string `mapstructure:"url" json:"url" yaml:"url"`
	Auth           string `mapstructure:"auth" json:"auth" yaml:"auth"`
	CollectionName string `mapstructure:"collection_name" json:"collection_name" yaml:"collection_name"`
}

type Ai struct {
	MaxPromptLength           int64    `mapstructure:"max_prompt_length" json:"max_prompt_length" yaml:"max_prompt_length"`
	MaxShortCodeLength        int64    `mapstructure:"max_short_code_length" json:"max_short_code_length" yaml:"max_short_code_length"`
	SemanticPrefix            string   `mapstructure:"semantic_prefix" json:"semantic_prefix" yaml:"semantic_prefix"`
	HybridPrefix              string   `mapstructure:"hybrid_prefix" json:"hybrid_prefix" yaml:"hybrid_prefix"`
	VectorSearchTopK          int64    `mapstructure:"vector_search_top_k" json:"vector_search_top_k" yaml:"vector_search_top_k"`
	VectorSimilarityThreshold float32  `mapstructure:"vector_similarity_threshold" json:"vector_similarity_threshold" yaml:"vector_similarity_threshold"`
	VectorSearchMinSimilarity float32  `mapstructure:"vector_search_min_similarity" json:"vector_search_min_similarity" yaml:"vector_search_min_similarity"`
	TriageContextQuestions    uint     `mapstructure:"triage_context_questions" json:"triage_context_questions" yaml:"triage_context_questions"`
	TriageMaxHistoryRounds    uint     `mapstructure:"triage_max_history_rounds" json:"triage_max_history_rounds" yaml:"triage_max_history_rounds"`
	TriageTimeout             int64    `mapstructure:"triage_timeout" json:"triage_timeout" yaml:"triage_timeout"`
	TransferGracePeriod       int64    `mapstructure:"transfer_grace_period" json:"transfer_grace_period" yaml:"transfer_grace_period"`
	HumanModeGracePeriod      int64    `mapstructure:"human_mode_grace_period" json:"human_mode_grace_period" yaml:"human_mode_grace_period"`
	ItemCardTTL               int64    `mapstructure:"item_card_ttl" json:"item_card_ttl" yaml:"item_card_ttl"`
	AsyncJobTimeout           int64    `mapstructure:"async_job_timeout" json:"async_job_timeout" yaml:"async_job_timeout"`
	ChatMaxHistoryRounds      uint     `mapstructure:"chat_max_history_rounds" json:"chat_max_history_rounds" yaml:"chat_max_history_rounds"`
	MaxAssistantPerRound      uint     `mapstructure:"max_assistant_per_round" json:"max_assistant_per_round" yaml:"max_assistant_per_round"`
	KeywordSyncInterval       uint     `mapstructure:"keyword_sync_interval" json:"keyword_sync_interval" yaml:"keyword_sync_interval"`
	KeywordReloadDebounce     uint     `mapstructure:"keyword_reload_debounce" json:"keyword_reload_debounce" yaml:"keyword_reload_debounce"`
	HistoryMaxAge             int64    `mapstructure:"history_max_age" json:"history_max_age" yaml:"history_max_age"`
	TransferMsgCooldown       int64    `mapstructure:"transfer_msg_cooldown" json:"transfer_msg_cooldown" yaml:"transfer_msg_cooldown"`
	TransferKeywords          []string `mapstructure:"transfer_keywords" json:"transfer_keywords" yaml:"transfer_keywords"`
}

type Oss struct {
	Endpoint        string `mapstructure:"endpoint" json:"endpoint" yaml:"endpoint"`
	AccessKeyId     string `mapstructure:"access_key_id" json:"access_key_id" yaml:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret" json:"access_key_secret" yaml:"access_key_secret"`
	Bucket          string `mapstructure:"bucket" json:"bucket" yaml:"bucket"`
	StoragePath     string `mapstructure:"storage_path" json:"storage_path" yaml:"storage_path"`
	CdnDomain       string `mapstructure:"cdn_domain" json:"cdn_domain" yaml:"cdn_domain"`
}
