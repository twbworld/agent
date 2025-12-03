package user

import (
	"errors"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"strings"
)

type Validator interface {
	ValidatorChatRequest(data *common.ChatRequest) error
}

type validator struct{}

func (v *validator) ValidatorChatRequest(data *common.ChatRequest) error {
	if data.Account.ID == 0 || data.Conversation.ID == 0 {
		return errors.New("参数错误[gftsd]")
	}
	// 消息内容和附件不能同时为空
	if strings.TrimSpace(data.Content) == "" && len(data.Attachments) == 0 {
		return errors.New("参数错误[gftsd]")
	}
	return nil
}
