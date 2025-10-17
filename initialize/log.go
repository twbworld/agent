package initialize

import (
	"fmt"
	"io"
	"os"

	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/sirupsen/logrus"
)

// InitLog 初始化logrus日志库
func (i *Initializer) InitLog() error {
	if err := utils.CreateFile(global.Config.RunLogPath); err != nil {
		return fmt.Errorf("创建文件错误[oirdtug]: %w", err)
	}

	global.Log = logrus.New()
	global.Log.SetFormatter(&logrus.JSONFormatter{})
	global.Log.SetLevel(logrus.InfoLevel)

	runfile, err := os.OpenFile(global.Config.RunLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开文件错误[0atrpf]: %w", err)
	}
	global.Log.SetOutput(io.MultiWriter(os.Stdout, runfile))
	i.logFileCloser = runfile // 存储文件关闭器
	return nil
}
