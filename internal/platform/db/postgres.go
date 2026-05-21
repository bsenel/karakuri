package db

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// postgresDialector builds the GORM dialect for PostgreSQL.
// The DSN follows the standard pq/pgx form, e.g.:
//
//	host=localhost port=5432 user=karakuri password=secret dbname=karakuri sslmode=disable
//	postgres://karakuri:secret@localhost:5432/karakuri?sslmode=disable
func postgresDialector(dsn string) gorm.Dialector {
	return postgres.Open(dsn)
}
