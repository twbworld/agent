package user

import (
	"context"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"gitee.com/taoJie_1/mall-agent/model/dto"
	"gitee.com/taoJie_1/mall-agent/service"
	"github.com/gin-gonic/gin"
)

type KeywordApi struct{}

func (k *KeywordApi) ListItems(c *gin.Context) {
	items, err := service.Service.UserServiceGroup.KeywordService.ListItems(c)
	if err != nil {
		common.Fail(c, err.Error())
		return
	}
	common.Success(c, items)
}

func (k *KeywordApi) UpsertItem(c *gin.Context) {
	var req dto.UpsertKnowledgeItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Fail(c, err.Error())
		return
	}

	if err := service.Service.UserServiceGroup.KeywordService.UpsertItem(c, &req); err != nil {
		common.Fail(c, err.Error())
		return
	}
	common.Success(c, nil)
}

func (k *KeywordApi) DeleteItem(c *gin.Context) {
	itemID := c.Param("id")
	if itemID == "" {
		common.Fail(c, "ID 不能为空")
		return
	}

	if err := service.Service.UserServiceGroup.KeywordService.DeleteItem(c, itemID); err != nil {
		common.Fail(c, err.Error())
		return
	}
	common.Success(c, nil)
}

func (k *KeywordApi) GenerateQuestions(c *gin.Context) {
	var req dto.GenerateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Fail(c, err.Error())
		return
	}

	resp, err := service.Service.UserServiceGroup.KeywordService.GenerateQuestions(c, &req)
	if err != nil {
		common.Fail(c, err.Error())
		return
	}
	common.Success(c, resp)
}

func (k *KeywordApi) ForceSync(c *gin.Context) {
	go func() {
		// 异步触发同步任务，避免阻塞请求。
		if err := service.Service.UserServiceGroup.KeywordService.ForceSync(context.Background()); err != nil {
			global.Log.Errorf("手动触发同步任务失败: %v", err)
		}
	}()
	common.Success(c, "同步任务已在后台触发")
}
