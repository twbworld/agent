package global

import (
	"fmt"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
)

func initChatwoot() error {
	client := chatwoot.NewClient(
		global.Config.Chatwoot.Url,
		int(global.Config.Chatwoot.AccountId),
		global.Config.Chatwoot.Auth,
		global.Log,
	)

	// 健康检查
	if _, err := client.GetCannedResponses(); err != nil {
		return fmt.Errorf("无法连接到Chatwoot服务 (url: %s): %w", global.Config.Chatwoot.Url, err)
	}

	global.ChatwootService = client
	return nil
}
