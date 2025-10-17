package controller

import "gitee.com/taoJie_1/mall-agent/controller/user"
import "gitee.com/taoJie_1/mall-agent/controller/admin"

var Api = new(ApiGroup)

type ApiGroup struct {
	UserApiGroup  user.ApiGroup  //用户前台
	AdminApiGroup admin.ApiGroup //管理后台
}
