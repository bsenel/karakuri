-- Phase 14: RBAC. The authoritative definition of these tables lives in the
-- auth/sql module (Store.Migrate), which the server runs at boot; this file
-- mirrors it for operators who apply schema changes by hand. Render the exact
-- statements the server would run with Store.DDL().

CREATE TABLE IF NOT EXISTS auth_principals (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT 'user',
	attrs_json  TEXT NOT NULL DEFAULT '{}',
	disabled    INTEGER NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS auth_credentials (
	principal_id  TEXT PRIMARY KEY,
	password_hash TEXT NOT NULL DEFAULT '',
	updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS auth_roles (
	name          TEXT PRIMARY KEY,
	description   TEXT NOT NULL DEFAULT '',
	inherits_json TEXT NOT NULL DEFAULT '[]',
	system        INTEGER NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS auth_policies (
	id              TEXT PRIMARY KEY,
	role_name       TEXT NOT NULL,
	ordinal         INTEGER NOT NULL DEFAULT 0,
	action          TEXT NOT NULL,
	resource        TEXT NOT NULL,
	effect          TEXT NOT NULL,
	conditions_json TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_auth_policies_role ON auth_policies (role_name);

CREATE TABLE IF NOT EXISTS auth_role_bindings (
	id           TEXT PRIMARY KEY,
	principal_id TEXT NOT NULL,
	role         TEXT NOT NULL,
	scope        TEXT NOT NULL DEFAULT '*'
);
CREATE INDEX IF NOT EXISTS idx_auth_bindings_principal ON auth_role_bindings (principal_id);

-- Timestamps are epoch milliseconds rather than native datetimes: drivers
-- disagree about how DATETIME round-trips, and this schema has to behave
-- identically on SQLite and Postgres.
CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
	id           TEXT PRIMARY KEY,
	family_id    TEXT NOT NULL,
	parent_id    TEXT NOT NULL DEFAULT '',
	principal_id TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	issued_at    BIGINT NOT NULL DEFAULT 0,
	expires_at   BIGINT NOT NULL DEFAULT 0,
	used_at      BIGINT,
	revoked_at   BIGINT
);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_family ON auth_refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_principal ON auth_refresh_tokens (principal_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_expires ON auth_refresh_tokens (expires_at);
