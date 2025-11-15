package user

type ServiceGroup struct {
	ActionService  ActionService
	LlmService     LlmService
	VectorService  VectorService
	HistoryService HistoryService
	Validator      Validator
}

func NewServiceGroup() ServiceGroup {
	return ServiceGroup{
		ActionService:  NewActionService(),
		LlmService:     NewLlmService(),
		VectorService:  NewVectorService(),
		HistoryService: NewHistoryService(),
		Validator:      &validator{},
	}
}
