package sql

import (
	"context"
	"strings"
)

// Migrate creates the tables and indices this store needs, if they are not
// already present. It is safe to call on every boot.
//
// The DDL is intentionally conservative: TEXT keys, integer timestamps
// (epoch milliseconds — see the note in store.go), and no foreign keys. Foreign
// keys are omitted because the auth tables frequently live alongside an
// application's own schema, sometimes in a database the application does not
// exclusively own; referential integrity is enforced in DeletePrincipal's
// transaction instead.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range s.schema() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// DDL returns the statements Migrate would run. Deployments that apply schema
// changes through a migration tool rather than at boot can render them with
// this instead of hand-copying the table definitions.
func (s *Store) DDL() string { return strings.Join(s.schema(), "\n\n") + "\n" }

func (s *Store) schema() []string {
	boolean := "BOOLEAN"
	if s.dialect == SQLite {
		// SQLite has no boolean type; INTEGER 0/1 is what its drivers return
		// either way, so declare what is actually stored.
		boolean = "INTEGER"
	}

	return []string{
		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_principals") + ` (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT 'user',
	attrs_json  TEXT NOT NULL DEFAULT '{}',
	disabled    ` + boolean + ` NOT NULL DEFAULT FALSE
)`,

		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_credentials") + ` (
	principal_id  TEXT PRIMARY KEY,
	password_hash TEXT NOT NULL DEFAULT '',
	updated_at    BIGINT NOT NULL DEFAULT 0
)`,

		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_roles") + ` (
	name          TEXT PRIMARY KEY,
	description   TEXT NOT NULL DEFAULT '',
	inherits_json TEXT NOT NULL DEFAULT '[]',
	system        ` + boolean + ` NOT NULL DEFAULT FALSE
)`,

		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_policies") + ` (
	id              TEXT PRIMARY KEY,
	role_name       TEXT NOT NULL,
	ordinal         INTEGER NOT NULL DEFAULT 0,
	action          TEXT NOT NULL,
	resource        TEXT NOT NULL,
	effect          TEXT NOT NULL,
	conditions_json TEXT NOT NULL DEFAULT '[]'
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.table("idx_auth_policies_role") +
			` ON ` + s.table("auth_policies") + ` (role_name)`,

		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_role_bindings") + ` (
	id           TEXT PRIMARY KEY,
	principal_id TEXT NOT NULL,
	role         TEXT NOT NULL,
	scope        TEXT NOT NULL DEFAULT '*'
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.table("idx_auth_bindings_principal") +
			` ON ` + s.table("auth_role_bindings") + ` (principal_id)`,

		`CREATE TABLE IF NOT EXISTS ` + s.table("auth_refresh_tokens") + ` (
	id           TEXT PRIMARY KEY,
	family_id    TEXT NOT NULL,
	parent_id    TEXT NOT NULL DEFAULT '',
	principal_id TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	issued_at    BIGINT NOT NULL DEFAULT 0,
	expires_at   BIGINT NOT NULL DEFAULT 0,
	used_at      BIGINT,
	revoked_at   BIGINT
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.table("idx_auth_refresh_family") +
			` ON ` + s.table("auth_refresh_tokens") + ` (family_id)`,
		`CREATE INDEX IF NOT EXISTS ` + s.table("idx_auth_refresh_principal") +
			` ON ` + s.table("auth_refresh_tokens") + ` (principal_id)`,
		`CREATE INDEX IF NOT EXISTS ` + s.table("idx_auth_refresh_expires") +
			` ON ` + s.table("auth_refresh_tokens") + ` (expires_at)`,
	}
}
