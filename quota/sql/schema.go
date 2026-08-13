package sql

import (
	"context"
	"strings"
)

// Migrate creates the tables and indices this backend needs, if they are not
// already present. It is safe to call on every boot.
//
// The DDL is conservative in the same way auth/sql's is: TEXT keys, integer
// timestamps in epoch milliseconds, and no foreign keys — these tables often
// live alongside an application's own schema in a database it does not
// exclusively own.
func (b *Backend) Migrate(ctx context.Context) error {
	for _, stmt := range b.schema() {
		if _, err := b.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// DDL returns the statements Migrate would run, for deployments that apply
// schema through a migration tool rather than at boot.
func (b *Backend) DDL() string { return strings.Join(b.schema(), "\n\n") + "\n" }

func (b *Backend) schema() []string {
	// SQLite has no native float type name in common use beyond REAL, and
	// Postgres wants DOUBLE PRECISION. This is the only column that differs.
	tokens := "DOUBLE PRECISION"
	if b.dialect == SQLite {
		tokens = "REAL"
	}

	return []string{
		// One row per key. The columns are the union of the three algorithms'
		// state, which keeps a take to a single row lock rather than a join —
		// the alternative, a table per algorithm, buys nothing because a key
		// only ever runs under one policy.
		`CREATE TABLE IF NOT EXISTS ` + b.table("quota_counters") + ` (
	key             TEXT PRIMARY KEY,
	algorithm       TEXT NOT NULL DEFAULT '',
	tokens          ` + tokens + ` NOT NULL DEFAULT 0,
	last_ms         BIGINT NOT NULL DEFAULT 0,
	window_start_ms BIGINT NOT NULL DEFAULT 0,
	used_units      BIGINT NOT NULL DEFAULT 0,
	expires_ms      BIGINT NOT NULL DEFAULT 0
)`,

		// SlidingLog only: one row per consumption, which is why the sliding
		// log is a poor fit for a high-frequency limit here and a fine one for
		// "a thousand a day". Costs are folded into n rather than repeated.
		`CREATE TABLE IF NOT EXISTS ` + b.table("quota_events") + ` (
	key   TEXT NOT NULL,
	at_ms BIGINT NOT NULL,
	n     BIGINT NOT NULL DEFAULT 1
)`,

		`CREATE INDEX IF NOT EXISTS ` + b.table("quota_events_key_at_idx") +
			` ON ` + b.table("quota_events") + ` (key, at_ms)`,

		// Sweeping is by expiry, so it wants its own index — without one, a
		// housekeeping pass table-scans every key on a busy deployment.
		`CREATE INDEX IF NOT EXISTS ` + b.table("quota_counters_expires_idx") +
			` ON ` + b.table("quota_counters") + ` (expires_ms)`,

		// One row per (subject, limit): an override replaces a named ceiling,
		// so a second row for the same pair would be a second answer with no
		// rule about which wins. The composite primary key is what makes the
		// upsert in PutOverride mean "replace".
		//
		// Deliberately not swept. A counter is worth expiring because it will
		// be recreated on the next request; an override is a decision somebody
		// made, and one that vanishes because nobody used it for a while is a
		// limit that silently drops back.
		`CREATE TABLE IF NOT EXISTS ` + b.table("quota_overrides") + ` (
	subject    TEXT NOT NULL,
	name       TEXT NOT NULL,
	cap_units  BIGINT NOT NULL DEFAULT 0,
	window_ms  BIGINT NOT NULL DEFAULT 0,
	expires_ms BIGINT NOT NULL DEFAULT 0,
	reason     TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (subject, name)
)`,

		// Requests are the audit trail: who asked for what, who decided, and
		// why. They outlive the override an approval writes, which is the point
		// — "why is this limit 5,000,000" is answered here long after somebody
		// has revoked it.
		`CREATE TABLE IF NOT EXISTS ` + b.table("quota_requests") + ` (
	id            TEXT PRIMARY KEY,
	subject       TEXT NOT NULL,
	name          TEXT NOT NULL,
	cap_units     BIGINT NOT NULL DEFAULT 0,
	window_ms     BIGINT NOT NULL DEFAULT 0,
	expires_ms    BIGINT NOT NULL DEFAULT 0,
	reason        TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT 'pending',
	requested_by  TEXT NOT NULL DEFAULT '',
	created_ms    BIGINT NOT NULL DEFAULT 0,
	decided_by    TEXT NOT NULL DEFAULT '',
	decided_ms    BIGINT NOT NULL DEFAULT 0,
	decision_note TEXT NOT NULL DEFAULT ''
)`,

		// The pending queue is the listing an approver opens, and it is the one
		// that must not table-scan as the decided ones accumulate.
		`CREATE INDEX IF NOT EXISTS ` + b.table("quota_requests_status_idx") +
			` ON ` + b.table("quota_requests") + ` (status, created_ms)`,

		`CREATE INDEX IF NOT EXISTS ` + b.table("quota_requests_subject_idx") +
			` ON ` + b.table("quota_requests") + ` (subject)`,
	}
}
