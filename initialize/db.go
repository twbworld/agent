package initialize

import (
	"fmt"
	"time"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type mysql struct{}
type sqlite struct{}

// dbStart 根据配置初始化数据库连接
func (i *Initializer) dbStart() error {
	var dbRes interface {
		connect() error
		version() string
	}

	switch global.Config.Database.Type {
	case string(enum.MYSQL):
		dbRes = &mysql{}
	case string(enum.SQLITE):
		dbRes = &sqlite{}
	default:
		dbRes = &sqlite{}
	}

	if err := dbRes.connect(); err != nil {
		return err
	}
	return nil
}

// dbClose 关闭数据库连接
func (i *Initializer) dbClose() error {
	if dao.DB != nil {
		return dao.DB.Close()
	}
	return  nil
}

func (s *sqlite) connect() error {
	var err error

	if dao.DB, err = sqlx.Open(string(enum.SQLITE), global.Config.Database.SqlitePath); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	if err = dao.DB.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	dao.DB.SetMaxOpenConns(16)
	dao.DB.SetMaxIdleConns(8)
	dao.DB.SetConnMaxLifetime(time.Minute * 5)

	if _, err = dao.DB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("数据库设置失败: %w", err)
	}
	if _, err = dao.DB.Exec("PRAGMA busy_timeout = 10000;"); err != nil {
		return fmt.Errorf("数据库设置失败: %w", err)
	}
	if _, err = dao.DB.Exec("PRAGMA synchronous = NORMAL;"); err != nil {
		return fmt.Errorf("数据库设置失败: %w", err)
	}

	dao.CanLock = false
	global.Log.Infof("%s版本: %s; 地址: %s", global.Config.Database.Type, s.version(), global.Config.Database.SqlitePath)
	return nil
}

func (m *mysql) connect() error {
	var err error
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", global.Config.Database.MysqlUsername, global.Config.Database.MysqlPassword, global.Config.Database.MysqlHost, global.Config.Database.MysqlPort, global.Config.Database.MysqlDbname)

	if dao.DB, err = sqlx.Connect(string(enum.MYSQL), dsn); err != nil {
		return fmt.Errorf("数据库连接失败[rwbhe3]: %s\n%w", dsn, err)
	}

	dao.DB.SetMaxOpenConns(16)
	dao.DB.SetMaxIdleConns(8)
	dao.DB.SetConnMaxLifetime(time.Minute * 5)

	if err = dao.DB.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %s\n%w", dsn, err)
	}

	dao.CanLock = true
	global.Log.Infof("%s版本: %s; 地址: @tcp(%s:%s)/%s", global.Config.Database.Type, m.version(), global.Config.Database.MysqlHost, global.Config.Database.MysqlPort, global.Config.Database.MysqlDbname)
	return nil
}

func (*sqlite) version() (t string) {
	if err := dao.DB.Get(&t, `SELECT sqlite_version()`); err != nil {
		global.Log.Warnf("查询sqlite版本失败: %v", err)
	}
	return
}

func (*mysql) version() (t string) {
	if err := dao.DB.Get(&t, `SELECT version()`); err != nil {
		global.Log.Warnf("查询mysql版本失败: %v", err)
	}
	return
}
