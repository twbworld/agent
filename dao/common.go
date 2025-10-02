package dao

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gitee.com/taoJie_1/chat/model/db"
)

type dbUtils struct{}

func (u *dbUtils) getBatchInsertSql(d db.Dbfunc, data []map[string]interface{}) (string, []interface{}, error) {
	if len(data) == 0 {
		return "", nil, nil
	}

	// 顺序
	keys := make([]string, 0, len(data[0]))
	for k := range data[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建字段
	var fields strings.Builder
	fields.WriteByte('(')
	for i, k := range keys {
		if i > 0 {
			fields.WriteString(", ")
		}
		fields.WriteByte('`')
		fields.WriteString(k)
		fields.WriteByte('`')
	}
	fields.WriteByte(')')

	valueStrings := make([]string, 0, len(data))
	valueArgs := make([]interface{}, 0, len(data)*len(keys))
	tags := db.GetBaseFieldDbTags()
	now := time.Now().Unix()

	for _, row := range data {
		if len(row) != len(keys) {
			return "", nil, fmt.Errorf("批量插入失败：数据行的字段数量不一致")
		}

		if tags.CreatedAtDbTag != "" {
			if _, exists := row[tags.CreatedAtDbTag]; !exists {
				row[tags.CreatedAtDbTag] = now
			}
		}
		if tags.UpdatedAtDbTag != "" {
			if _, exists := row[tags.UpdatedAtDbTag]; !exists {
				row[tags.UpdatedAtDbTag] = now
			}
		}

		// 构建 VALUES 子句中的单行占位符, e.g., "(?, ?, ?)"
		valueStrings = append(valueStrings, "(?"+strings.Repeat(", ?", len(keys)-1)+")")

		// 按照排序后的字段顺序，添加参数到 valueArgs
		for _, k := range keys {
			val, ok := row[k]
			if !ok {
				return "", nil, fmt.Errorf("批量插入失败：数据行缺少字段 '%s'", k)
			}
			valueArgs = append(valueArgs, val)
		}
	}

	var sql strings.Builder
	sql.WriteString("INSERT INTO `")
	sql.WriteString(d.TableName())
	sql.WriteString("` ")
	sql.WriteString(fields.String())
	sql.WriteString(" VALUES ")
	sql.WriteString(strings.Join(valueStrings, ", "))

	return sql.String(), valueArgs, nil
}

func (u *dbUtils) getUpdateSql(d db.Dbfunc, id uint, data map[string]interface{}) (string, []interface{}) {
	if len(data) < 1 {
		return ``, []interface{}{}
	}

	var (
		fields strings.Builder
		sql    strings.Builder
		args   []interface{} = make([]interface{}, 0, len(data))
	)

	for k, v := range data {
		fields.WriteString(" `")
		fields.WriteString(k)
		fields.WriteString("` = ?,")
		args = append(args, v)
	}

	sql.WriteString("UPDATE `")
	sql.WriteString(d.TableName())
	sql.WriteString("` SET")
	sql.WriteString(strings.TrimRight(fields.String(), ","))
	sql.WriteString(" WHERE `id` = ?")
	args = append(args, id)

	return sql.String(), args
}
