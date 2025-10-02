package controller

import "gitee.com/taoJie_1/chat/controller/user"
import "gitee.com/taoJie_1/chat/controller/admin"

var Api = new(ApiGroup)

type ApiGroup struct {
	UserApiGroup  user.ApiGroup
	AdminApiGroup admin.ApiGroup
}
