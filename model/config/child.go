package config

type Mcp struct {
	Url  string `json:"url" mapstructure:"url" yaml:"url"`
	Auth string `json:"auth" mapstructure:"auth" yaml:"auth"`
}

type Database struct {
	Type          string `json:"type" mapstructure:"type" yaml:"type"`
	SqlitePath    string `json:"sqlite_path" mapstructure:"sqlite_path" yaml:"sqlite_path"`
	MysqlHost     string `json:"mysql_host" mapstructure:"mysql_host" yaml:"mysql_host"`
	MysqlPort     string `json:"mysql_port" mapstructure:"mysql_port" yaml:"mysql_port"`
	MysqlDbname   string `json:"mysql_dbname" mapstructure:"mysql_dbname" yaml:"mysql_dbname"`
	MysqlUsername string `json:"mysql_username" mapstructure:"mysql_username" yaml:"mysql_username"`
	MysqlPassword string `json:"mysql_password" mapstructure:"mysql_password" yaml:"mysql_password"`
}

type Redis struct {
	Addr                   string `json:"addr" mapstructure:"addr" yaml:"addr"`
	Password               string `json:"password" mapstructure:"password" yaml:"password"`
	DB                     int64  `json:"db" mapstructure:"db" yaml:"db"`
	LockExpiry             int64  `json:"lock_expiry" mapstructure:"lock_expiry" yaml:"lock_expiry"`
	ConversationHistoryTTL int64  `json:"conversation_history_ttl" mapstructure:"conversation_history_ttl" yaml:"conversation_history_ttl"`
	HistoryLockExpiry      int64  `json:"history_lock_expiry" mapstructure:"history_lock_expiry" yaml:"history_lock_expiry"`
}

type Chatwoot struct {
	Url       string `yaml:"url" mapstructure:"url" json:"url"`
	AccountId int64  `yaml:"account_id" mapstructure:"account_id" json:"account_id"`
	Auth      string `yaml:"auth" mapstructure:"auth" json:"auth"`
	BotAuth   string `yaml:"bot_auth" mapstructure:"bot_auth" json:"bot_auth"`
}

type modelConfig struct {
	Url     string `json:"url" mapstructure:"url" yaml:"url"`
	Model   string `json:"model" mapstructure:"model" yaml:"model"`
	Auth    string `json:"auth" mapstructure:"auth" yaml:"auth"`
	Timeout int64  `json:"timeout" mapstructure:"timeout" yaml:"timeout"`
}

type Llm struct {
	modelConfig `mapstructure:",squash"`
	Size        string   `json:"size" mapstructure:"size" yaml:"size"`
	Temperature *float32 `json:"temperature,omitempty" mapstructure:"temperature" yaml:"temperature,omitempty"`
}

type LlmEmbedding struct {
	modelConfig  `mapstructure:",squash"`
	BatchTimeout int64 `mapstructure:"batch_timeout" json:"batch_timeout" yaml:"batch_timeout"`
}

type VectorDb struct {
	Url            string `json:"url" mapstructure:"url" yaml:"url"`
	Auth           string `json:"auth" mapstructure:"auth" yaml:"auth"`
	CollectionName string `json:"collection_name" mapstructure:"collection_name" yaml:"collection_name"`
}

type Ai struct {
	MaxPromptLength           int64    `json:"max_prompt_length" mapstructure:"max_prompt_length" yaml:"max_prompt_length"`
	MaxShortCodeLength        int64    `json:"max_short_code_length" mapstructure:"max_short_code_length" yaml:"max_short_code_length"`
	SemanticPrefix            string   `json:"semantic_prefix" mapstructure:"semantic_prefix" yaml:"semantic_prefix"`
	HybridPrefix              string   `json:"hybrid_prefix" mapstructure:"hybrid_prefix" yaml:"hybrid_prefix"`
	VectorSearchTopK          int64    `json:"vector_search_top_k" mapstructure:"vector_search_top_k" yaml:"vector_search_top_k"`
	VectorSimilarityThreshold float32  `json:"vector_similarity_threshold" mapstructure:"vector_similarity_threshold" yaml:"vector_similarity_threshold"`
	VectorSearchMinSimilarity float32  `json:"vector_search_min_similarity" mapstructure:"vector_search_min_similarity" yaml:"vector_search_min_similarity"`
	TriageContextQuestions    uint     `json:"triage_context_questions" mapstructure:"triage_context_questions" yaml:"triage_context_questions"`
	TransferGracePeriod       int64    `json:"transfer_grace_period" mapstructure:"transfer_grace_period" yaml:"transfer_grace_period"`
	AsyncJobTimeout           int64    `json:"async_job_timeout" mapstructure:"async_job_timeout" yaml:"async_job_timeout"`
	TransferKeywords          []string `json:"transfer_keywords" mapstructure:"transfer_keywords" yaml:"transfer_keywords"`
	MaxLlmHistoryMessages     uint     `json:"max_llm_history_messages" mapstructure:"max_llm_history_messages" yaml:"max_llm_history_messages"`
}
