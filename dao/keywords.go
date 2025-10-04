package dao

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/db"
	"gitee.com/taoJie_1/chat/model/enum"
	"gitee.com/taoJie_1/chat/pkg/chatwoot"
	"github.com/jmoiron/sqlx"
)

type KeywordsDb struct{}

// 获取所有数据
func (d *KeywordsDb) GetKeywordsAllList(list *[]common.KeywordsList, tx ...*sqlx.Tx) error {
	sql := fmt.Sprintf("SELECT `short_code`, `content` FROM `%s` GROUP BY `short_code` ORDER BY id DESC;", db.Keywords{}.TableName())

	if len(tx) > 0 && tx[0] != nil {
		return tx[0].Select(list, sql)
	}
	return DB.Select(list, sql)
}

// 清空表
func (d *KeywordsDb) CleanTable(tx *sqlx.Tx) error {
	if tx == nil {
		return errors.New("请使用事务[ioddfsaa]")
	}

	switch global.Config.Database.Type {
	case string(enum.SQLITE):
		sql := fmt.Sprintf("DELETE FROM `%s`", db.Keywords{}.TableName())
		_, err := tx.Exec(sql)

		if err != nil {
			return err
		}
		// 重置自增ID
		sql = fmt.Sprintf("DELETE FROM sqlite_sequence WHERE name='%s'", db.Keywords{}.TableName())
		_, err = tx.Exec(sql)
		return err
	case string(enum.MYSQL):
		sql := fmt.Sprintf("TRUNCATE TABLE `%s`", db.Keywords{}.TableName())
		_, err := tx.Exec(sql)
		return err
	}

	return errors.New("数据库类型错误[rjfsos]")
}

// 插入数据
func (d *KeywordsDb) BatchInsert(data []chatwoot.CannedResponse, tx *sqlx.Tx) (int64, error) {
	if tx == nil {
		return 0, errors.New("请使用事务[ioddfsaa]")
	}

	if len(data) == 0 {
		return 0, nil
	}

	var sqlData []map[string]interface{}
	for _, resp := range data {
		resp.ShortCode = strings.TrimSpace(resp.ShortCode)
		if resp.ShortCode == "" || resp.Content == "" {
			continue // 跳过无效数据
		}
		if utf8.RuneCountInString(resp.ShortCode) > int(global.Config.Ai.MaxShortCodeLength) {
			global.Log.Warnf("short_code 超出长度限制，已跳过: %s", resp.ShortCode)
			continue
		}

		sqlData = append(sqlData, map[string]interface{}{
			"short_code": resp.ShortCode,
			"content":    resp.Content,
			"account_id": resp.AccountId,
		})
	}

	sql, args, err := utils.getBatchInsertSql(db.Keywords{}, sqlData)
	if err != nil {
		return 0, fmt.Errorf("构建批量插入SQL失败: %w", err)
	}

	sql = tx.Rebind(sql)
	result, err := tx.Exec(sql, args...)
	if err != nil {
		return 0, fmt.Errorf("批量插入数据失败: %w", err)
	}

	return result.RowsAffected()
}
