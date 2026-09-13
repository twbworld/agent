package controller

import "github.com/twbworld/agent/controller/user"
import "github.com/twbworld/agent/controller/admin"

var Api = new(ApiGroup)

type ApiGroup struct {
	UserApiGroup  user.ApiGroup  //用户前台
	AdminApiGroup admin.ApiGroup //管理后台
}
