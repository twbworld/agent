package common

import (
)

type KeywordsList struct {
	ShortCode string `db:"short_code" json:"short_code"`
	Content   string `db:"content" json:"content"`
}
