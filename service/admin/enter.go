package admin

import "github.com/twbworld/agent/task"

type ServiceGroup struct {
	KeywordService   KeywordService
	UploadService    UploadService
	DashboardService DashboardService
}

func NewServiceGroup(taskManager *task.Manager) ServiceGroup {
	return ServiceGroup{
		KeywordService:   NewKeywordService(taskManager),
		UploadService:    NewUploadService(),
		DashboardService: NewDashboardService(),
	}
}
