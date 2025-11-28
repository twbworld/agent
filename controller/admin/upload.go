package admin

import (
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/service"
	"github.com/gin-gonic/gin"
)

type UploadApi struct{}

// UploadApi 处理管理面板的图片上传
func (u *UploadApi) UploadImage(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		common.Fail(c, "获取文件失败: "+err.Error())
		return
	}

	url, err := service.Service.AdminServiceGroup.UploadService.UploadImage(file)
	if err != nil {
		common.Fail(c, "上传失败: "+err.Error())
		return
	}

	common.Success(c, gin.H{"url": url})
}
