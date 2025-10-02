package db

import (
	"reflect"
	"sync"

	"gitee.com/taoJie_1/chat/utils"
)

// 所有数据库结构体 都需实现的接口
type Dbfunc interface {
	TableName() string
}

// 可能为null的字段, 用指针
type BaseField struct {
	Id        uint  `db:"id" json:"id"`
	CreatedAt int64 `db:"created_at" json:"created_at"`
	UpdatedAt int64 `db:"updated_at" json:"-"`
}

func (b *BaseField) CreatedAtFormat() string {
	return utils.TimeFormat(b.CreatedAt)
}

func (b *BaseField) UpdateAtFormat() string {
	return utils.TimeFormat(b.UpdatedAt)
}

var (
	once sync.Once

	baseFieldInfo struct {
		CreatedAtDbTag string
		UpdatedAtDbTag string
	}
)

func GetBaseFieldDbTags() struct {
	CreatedAtDbTag string
	UpdatedAtDbTag string
} {
	once.Do(func() {
		t := reflect.TypeOf(BaseField{})

		if field, found := t.FieldByName("CreatedAt"); found {
			baseFieldInfo.CreatedAtDbTag = field.Tag.Get("db")
		}
		if field, found := t.FieldByName("UpdatedAt"); found {
			baseFieldInfo.UpdatedAtDbTag = field.Tag.Get("db")
		}
	})
	return baseFieldInfo
}
