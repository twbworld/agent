package config

type Database struct {
	Type          string `json:"type" mapstructure:"type" yaml:"type"`
	SqlitePath    string `json:"sqlite_path" mapstructure:"sqlite_path" yaml:"sqlite_path"`
	MysqlHost     string `json:"mysql_host" mapstructure:"mysql_host" yaml:"mysql_host"`
	MysqlPort     string `json:"mysql_port" mapstructure:"mysql_port" yaml:"mysql_port"`
	MysqlDbname   string `json:"mysql_dbname" mapstructure:"mysql_dbname" yaml:"mysql_dbname"`
	MysqlUsername string `json:"mysql_username" mapstructure:"mysql_username" yaml:"mysql_username"`
	MysqlPassword string `json:"mysql_password" mapstructure:"mysql_password" yaml:"mysql_password"`
}

type Chatwoot struct {
	Url       string `json:"url" mapstructure:"url" yaml:"url"`
	AccountId int64  `json:"account_id" mapstructure:"account_id" yaml:"account_id"`
	Auth      string `json:"auth" mapstructure:"auth" yaml:"auth"`
}

type Llm struct {
	Url          string `json:"url" mapstructure:"url" yaml:"url"`
	Model        string `json:"model" mapstructure:"model" yaml:"model"`
	Auth         string `json:"auth" mapstructure:"auth" yaml:"auth"`
	Size         string `json:"size" mapstructure:"size" yaml:"size"`
	Timeout      int64  `json:"timeout" mapstructure:"timeout" yaml:"timeout"`
	EmbeddingDim int64  `json:"embedding_dim" mapstructure:"embedding_dim" yaml:"embedding_dim"`
}

type VectorDb struct {
	Url            string `json:"url" mapstructure:"url" yaml:"url"`
	Auth           string `json:"auth" mapstructure:"auth" yaml:"auth"`
	CollectionName string `json:"collection_name" mapstructure:"collection_name" yaml:"collection_name"`
}

type Ai struct {
	MaxPromptLength           uint     `json:"max_prompt_length" mapstructure:"max_prompt_length" yaml:"max_prompt_length"`
	MaxShortCodeLength        uint     `json:"max_short_code_length" mapstructure:"max_short_code_length" yaml:"max_short_code_length"`
	SemanticPrefix            string   `json:"semantic_prefix" mapstructure:"semantic_prefix" yaml:"semantic_prefix"`
	ExactPrefix               string   `json:"exact_prefix" mapstructure:"exact_prefix" yaml:"exact_prefix"`
	TransferKeywords          []string `json:"transfer_keywords" mapstructure:"transfer_keywords" yaml:"transfer_keywords"`
	VectorSearchTopK          int      `json:"vector_search_top_k" mapstructure:"vector_search_top_k" yaml:"vector_search_top_k"`
	VectorSimilarityThreshold float32  `json:"vector_similarity_threshold" mapstructure:"vector_similarity_threshold" yaml:"vector_similarity_threshold"`
	VectorSearchMinSimilarity float32  `json:"vector_search_min_similarity" mapstructure:"vector_search_min_similarity" yaml:"vector_search_min_similarity"`
	AsyncJobTimeout           int64    `json:"async_job_timeout" mapstructure:"async_job_timeout" yaml:"async_job_timeout"`
}
