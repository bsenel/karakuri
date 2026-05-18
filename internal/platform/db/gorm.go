package db

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		dialector = sqliteDialector(dsn)
	case "postgres":
		return nil, fmt.Errorf("postgres: %w", errNotImpl)
	case "mysql":
		return nil, fmt.Errorf("mysql: %w", errNotImpl)
	default:
		return nil, fmt.Errorf("unknown driver: %s", driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

var errNotImpl = fmt.Errorf("not implemented")
