package user

import "gitee.com/taoJie_1/mall-agent/task"

type ServiceGroup struct {
	ActionService    ActionService
	LlmService       LlmService
	VectorService    VectorService
	HistoryService   HistoryService
	DashboardService DashboardService
	Validator        Validator
}

func NewServiceGroup(taskManager *task.Manager) ServiceGroup {
	return ServiceGroup{
		ActionService:    NewActionService(),
		LlmService:       NewLlmService(),
		VectorService:    NewVectorService(),
		HistoryService:   NewHistoryService(),
		DashboardService: NewDashboardService(),
		Validator:        &validator{},
	}
}
