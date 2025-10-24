package user

type ServiceGroup struct {
	ActionService ActionService
	LlmService    LlmService
	VectorService VectorService
	Validator     Validator
}

func NewServiceGroup() ServiceGroup {
	return ServiceGroup{
		ActionService: NewActionService(),
		LlmService:    NewLlmService(),
		VectorService: NewVectorService(),
		Validator:     &validator{},
	}
}
