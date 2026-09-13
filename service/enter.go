package service

import (
	"github.com/twbworld/agent/service/admin"
	"github.com/twbworld/agent/service/common"
	"github.com/twbworld/agent/service/user"
)

type ServiceGroup struct {
	UserServiceGroup   user.ServiceGroup
	AdminServiceGroup  admin.ServiceGroup
	CommonServiceGroup common.ServiceGroup
}

var Service = new(ServiceGroup)
