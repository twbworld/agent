package task

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitee.com/taoJie_1/mall-agent/global"
)

// 清除日志文件
func (m *Manager) CleanUpLogs() error {
	retentionDays := global.Config.LogRetentionDays
	if retentionDays == 0 {
		global.Log.Info("日志清理功能已禁用 (log_retention_days = 0)")
		return nil
	}

	global.Log.Info("开始执行日志清理任务...")

	// 假设 gin_log_path 和 run_log_path 在同一个目录下
	logDir := filepath.Dir(global.Config.RunLogPath)
	now := time.Now().In(global.Tz)
	// 当天的零点
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, global.Tz)
	// 计算截止日期
	cutoffDate := today.AddDate(0, 0, -int(retentionDays))

	deletedCount := 0
	var errors []string

	err := filepath.WalkDir(logDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// 从文件名中解析日期, e.g., run.log.2025-10-28
		fileDate, ok := parseDateFromLogFileName(d.Name())
		if !ok {
			return nil // 不是带日期的日志文件，跳过
		}

		// 如果文件日期在截止日期之前，则删除
		if fileDate.Before(cutoffDate) {
			if err := os.Remove(path); err != nil {
				errMsg := fmt.Sprintf("删除旧日志文件 %s 失败: %v", path, err)
				global.Log.Error(errMsg)
				errors = append(errors, errMsg)
			} else {
				global.Log.Infof("已删除旧日志文件: %s", path)
				deletedCount++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("遍历日志目录 '%s' 失败: %w", logDir, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("日志清理过程中发生错误: %s", strings.Join(errors, "; "))
	}

	global.Log.Infof("日志清理任务完成，共删除 %d 个文件", deletedCount)
	return nil
}

// parseDateFromLogFileName 从日志文件名中解析日期
// 文件名格式如: gin.log.2025-10-28, run.log.2025-10-28
func parseDateFromLogFileName(filename string) (time.Time, bool) {
	parts := strings.Split(filename, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}

	// 日期部分应在最后
	dateStr := parts[len(parts)-1]
	// 使用 "2006-01-02" 格式解析
	t, err := time.ParseInLocation("2006-01-02", dateStr, global.Tz)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
