package config

// viper要用到mapstructure/yaml
type Config struct {
	Debug        bool         `json:"debug" mapstructure:"debug" yaml:"debug"`
	ProjectName  string       `json:"project_name" mapstructure:"project_name" yaml:"project_name"`
	GinAddr      string       `json:"gin_addr" mapstructure:"gin_addr" yaml:"gin_addr"`
	Domain       string       `json:"domain" mapstructure:"domain" yaml:"domain"`
	StaticDir    string       `json:"static_dir" mapstructure:"static_dir" yaml:"static_dir"`
	GinLogPath   string       `json:"gin_log_path" mapstructure:"gin_log_path" yaml:"gin_log_path"`
	RunLogPath   string       `json:"run_log_path" mapstructure:"run_log_path" yaml:"run_log_path"`
	Tz           string       `json:"tz" mapstructure:"tz" yaml:"tz"`
	Cors         []string     `json:"cors" mapstructure:"cors" yaml:"cors"`
	Database     Database     `json:"database" mapstructure:"database" yaml:"database"`
	Chatwoot     Chatwoot     `json:"chatwoot" mapstructure:"chatwoot" yaml:"chatwoot"`
	Llm          []Llm        `json:"llm" mapstructure:"llm" yaml:"llm"`
	LlmEmbedding LlmEmbedding `json:"llm_embedding" mapstructure:"llm_embedding" yaml:"llm_embedding"`
	VectorDb     VectorDb     `json:"vector_db" mapstructure:"vector_db" yaml:"vector_db"`
	Ai           Ai           `json:"ai" mapstructure:"ai" yaml:"ai"`
}
