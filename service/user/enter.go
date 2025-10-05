package user

type ServiceGroup struct {
	ActionService IActionService
	LlmService    ILlmService
	VectorService IVectorService
	Validator     IValidator
}

func NewServiceGroup() ServiceGroup {
	return ServiceGroup{
		ActionService: NewActionService(),
		LlmService:    NewLlmService(),
		VectorService: NewVectorService(),
		Validator:     &Validator{},
	}
}
