package db

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func sqliteDialector(dsn string) gorm.Dialector {
	return sqlite.Open(dsn)
}
