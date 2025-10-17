package service

import (
	"gitee.com/taoJie_1/mall-agent/service/admin"
	"gitee.com/taoJie_1/mall-agent/service/common"
	"gitee.com/taoJie_1/mall-agent/service/user"
)

type ServiceGroup struct {
	UserServiceGroup   user.ServiceGroup
	AdminServiceGroup  admin.ServiceGroup
	CommonServiceGroup common.ServiceGroup
}

var Service = new(ServiceGroup)
