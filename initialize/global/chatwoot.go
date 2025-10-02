package global

import (
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
)

// 初始化向量化服务
func (*GlobalInit) initChatwoot() error {
	global.ChatwootClient = chatwoot.NewClient(
		global.Config.Chatwoot.Url,
		int(global.Config.Chatwoot.AccountId),
		global.Config.Chatwoot.Auth,
		global.Log,
	)
	return nil
}
