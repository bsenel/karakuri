package auth

import (
	"fmt"

	authsql "github.com/bsenel/karakuri/auth/sql"
	"gorm.io/gorm"
)

// NewStore builds the auth store on top of Karakuri's existing database
// connection.
//
// GORM hands out the underlying *sql.DB, which is exactly what auth/sql wants —
// so the auth module never learns about GORM, and Karakuri does not open a
// second pool against the same database.
func NewStore(db *gorm.DB) (*authsql.Store, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("auth: take *sql.DB from GORM: %w", err)
	}
	dialect := authsql.SQLite
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		dialect = authsql.Postgres
	}
	return authsql.New(sqlDB, authsql.Options{Dialect: dialect})
}
