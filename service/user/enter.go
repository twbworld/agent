package user

// ServiceGroup 聚合了所有用户相关的服务
type ServiceGroup struct {
	Validator
	ActionService // ActionService 包含了如转人工、关键词匹配等逻辑
	LlmService    // LlmService 封装了与大模型交互的业务逻辑
}

// NewServiceGroup 创建并返回一个完整的、已初始化的用户服务组
func NewServiceGroup() ServiceGroup {
	return ServiceGroup{
		ActionService: *NewActionService(),
		LlmService:    *NewLlmService(),
	}
}
