package service

import (
	"gitee.com/taoJie_1/chat/service/admin"
	"gitee.com/taoJie_1/chat/service/common"
	"gitee.com/taoJie_1/chat/service/user"
)

type ServiceGroup struct {
	UserServiceGroup   user.ServiceGroup
	AdminServiceGroup  admin.ServiceGroup
	CommonServiceGroup common.ServiceGroup
}

var Service = new(ServiceGroup)
