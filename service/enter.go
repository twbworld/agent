package service

import (
	"gitee.com/taoJie_1/chat/service/admin"
	"gitee.com/taoJie_1/chat/service/user"
)

var Service = new(ServiceGroup)

type ServiceGroup struct {
	UserServiceGroup  user.ServiceGroup
	AdminServiceGroup admin.ServiceGroup
}
