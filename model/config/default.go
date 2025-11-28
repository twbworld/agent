package config

import "encoding/json"

type Config struct {
	Debug            bool           `mapstructure:"debug" json:"debug" yaml:"debug"`
	ProjectName      string         `mapstructure:"project_name" json:"project_name" yaml:"project_name"`
	GinAddr          string         `mapstructure:"gin_addr" json:"gin_addr" yaml:"gin_addr"`
	Domain           string         `mapstructure:"domain" json:"domain" yaml:"domain"`
	StaticDir        string         `mapstructure:"static_dir" json:"static_dir" yaml:"static_dir"`
	GinLogPath       string         `mapstructure:"gin_log_path" json:"gin_log_path" yaml:"gin_log_path"`
	RunLogPath       string         `mapstructure:"run_log_path" json:"run_log_path" yaml:"run_log_path"`
	LogRetentionDays uint           `mapstructure:"log_retention_days" json:"log_retention_days" yaml:"log_retention_days"`
	Tz               string         `mapstructure:"tz" json:"tz" yaml:"tz"`
	Cors             []string       `mapstructure:"cors" json:"cors" yaml:"cors"`
	Database         Database       `mapstructure:"database" json:"database" yaml:"database"`
	Redis            Redis          `mapstructure:"redis" json:"redis" yaml:"redis"`
	Chatwoot         Chatwoot       `mapstructure:"chatwoot" json:"chatwoot" yaml:"chatwoot"`
	Llm              []Llm          `mapstructure:"llm" json:"llm" yaml:"llm"`
	LlmEmbedding     LlmEmbedding   `mapstructure:"llm_embedding" json:"llm_embedding" yaml:"llm_embedding"`
	VectorDb         VectorDb       `mapstructure:"vector_db" json:"vector_db" yaml:"vector_db"`
	Ai               Ai             `mapstructure:"ai" json:"ai" yaml:"ai"`
	McpServers       map[string]Mcp `mapstructure:"mcp_servers" json:"mcp_servers" yaml:"mcp_servers"`
	Oss              Oss            `mapstructure:"oss" json:"oss" yaml:"oss"`
}

// DeepCopy 使用JSON序列化和反序列化实现Config对象的深度拷贝
func (c *Config) DeepCopy() *Config {
	newConfig := new(Config)
	data, _ := json.Marshal(c)
	_ = json.Unmarshal(data, newConfig)
	return newConfig
}
