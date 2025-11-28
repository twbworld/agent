package admin

import "gitee.com/taoJie_1/mall-agent/task"

type ServiceGroup struct {
	KeywordService KeywordService
	UploadService  UploadService
}

func NewServiceGroup(taskManager *task.Manager) ServiceGroup {
	return ServiceGroup{
		KeywordService: NewKeywordService(taskManager),
		UploadService:  NewUploadService(),
	}
}
