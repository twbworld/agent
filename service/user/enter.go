package user

type ServiceGroup struct {
	Validator
	ActionService
	LlmService
}

// NewServiceGroup 创建并返回一个完整的、已初始化的用户服务组
func NewServiceGroup() ServiceGroup {
	return ServiceGroup{
		ActionService: *NewActionService(),
		LlmService:    *NewLlmService(),
	}
}
