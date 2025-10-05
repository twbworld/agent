package user

import (
	"errors"
	"gitee.com/taoJie_1/chat/model/common"
	"strings"
)

type IValidator interface {
	ValidatorChatRequest(data *common.ChatRequest) error
}

type Validator struct{}

func (v *Validator) ValidatorChatRequest(data *common.ChatRequest) error {
	if data.Conversation.AccountID == 0 || data.Conversation.ConversationID == 0 || strings.TrimSpace(data.Content) == "" {
		return errors.New("参数错误[gftsd]")
	}
	return nil
}
