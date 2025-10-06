package global

import (
	"fmt"
	"time"

	"gitee.com/taoJie_1/chat/global"
)

func (*GlobalInit) InitTz() error {
	Location, err := time.LoadLocation(global.Config.Tz)
	if err != nil {
		return fmt.Errorf("时区配置失败[siortuj]: %w", err)
	}
	global.Tz = Location
	return nil
}
