package global

import (
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
)

func initChatwoot() error {
	global.ChatwootService = chatwoot.NewClient(
		global.Config.Chatwoot.Url,
		int(global.Config.Chatwoot.AccountId),
		global.Config.Chatwoot.Auth,
		global.Log,
	)
	return nil
}
