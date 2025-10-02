package system

import (
	"fmt"
	"time"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/enum"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type mysql struct{}
type sqlite struct{}
type class interface {
	connect() error
	version() string
}

func DbStart() error {
	var dbRes class

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

// 关闭数据库连接
func DbClose() error {
	if dao.DB != nil {
		return dao.DB.Close()
	}
	return nil
}

// 连接SQLite数据库
func (s *sqlite) connect() error {
	var err error

	if dao.DB, err = sqlx.Open(string(enum.SQLITE), global.Config.Database.SqlitePath); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	//没有数据库会创建
	if err = dao.DB.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	dao.DB.SetMaxOpenConns(16)
	dao.DB.SetMaxIdleConns(8)
	dao.DB.SetConnMaxLifetime(time.Minute * 5)

	//提高并发
	if _, err = dao.DB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("数据库设置失败: %w", err)
	}
	//超时等待
	if _, err = dao.DB.Exec("PRAGMA busy_timeout = 10000;"); err != nil {
		return fmt.Errorf("数据库设置失败: %w", err)
	}
	// 设置同步模式为 NORMAL
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

	//也可以使用MustConnect连接不成功就panic
	if dao.DB, err = sqlx.Connect(string(enum.MYSQL), dsn); err != nil {
		return fmt.Errorf("数据库连接失败[rwbhe3]: %s\n%w", dsn, err)
	}

	dao.DB.SetMaxOpenConns(16)
	dao.DB.SetMaxIdleConns(8)
	dao.DB.SetConnMaxLifetime(time.Minute * 5) // 设置连接的最大生命周期

	if err = dao.DB.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %s\n%w", dsn, err)
	}

	dao.CanLock = true
	global.Log.Infof("%s版本: %s; 地址: @tcp(%s:%s)/%s", global.Config.Database.Type, m.version(), global.Config.Database.MysqlHost, global.Config.Database.MysqlPort, global.Config.Database.MysqlDbname)
	return nil
}

func (*sqlite) version() (t string) {
	dao.DB.Get(&t, `SELECT sqlite_version()`)
	return
}

func (*mysql) version() (t string) {
	dao.DB.Get(&t, `SELECT version()`)
	return
}
