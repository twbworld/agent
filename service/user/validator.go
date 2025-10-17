package user

import (
	"errors"
	"gitee.com/taoJie_1/mall-agent/model/common"
	"strings"
)

type IValidator interface {
	ValidatorChatRequest(data *common.ChatRequest) error
}

type Validator struct{}

func (v *Validator) ValidatorChatRequest(data *common.ChatRequest) error {
	if data.Conversation.AccountID == 0 || data.Conversation.ConversationID == 0 {
		return errors.New("参数错误[gftsd]")
	}
	// 消息内容和附件不能同时为空
	if strings.TrimSpace(data.Content) == "" && len(data.Attachments) == 0 {
		return errors.New("参数错误[gftsd]")
	}
	return nil
}
