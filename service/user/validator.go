package user

import (
	"errors"
	"strings"
	"unicode/utf8"


	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
)

type Validator struct{}

// 检验ChatRequest参数
func (v *Validator) ValidatorChatRequest(data *common.ChatRequest) error {
	if data.Conversation.AccountID == 0 || data.Conversation.ConversationID == 0 || strings.TrimSpace(data.Content) == "" {
		return errors.New("参数错误[gftsd]")
	}
	if utf8.RuneCountInString(data.Content) > int(global.Config.Ai.MaxPromptLength) {
		return errors.New("提问内容过长[dois]")
	}
	return nil
}
