package db

type Keywords struct {
	BaseField
	ShortCode string `db:"short_code" json:"short_code" info:"关键词"`
	Content   string `db:"content" json:"content" info:"内容"`
	AccountId uint   `db:"account_id" json:"account_id" info:"账号id"`
}

func (Keywords) TableName() string {
	return `keywords`
}
