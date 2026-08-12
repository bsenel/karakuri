package sql

import (
	"context"
	stdsql "database/sql"
	"strings"
)

// Migrate creates the tables and indices this ledger needs, if they are not
// already present. Safe to call on every boot.
//
// Conservative in the same way quota/sql's is: TEXT keys, integer timestamps in
// epoch milliseconds, and no foreign keys — these tables often live alongside
// an application's own schema in a database it does not exclusively own.
func (l *Ledger) Migrate(ctx context.Context) error {
	for _, stmt := range l.schema() {
		if _, err := l.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// DDL returns the statements Migrate would run, for deployments that apply
// schema through a migration tool.
func (l *Ledger) DDL() string { return strings.Join(l.schema(), "\n\n") + "\n" }

func (l *Ledger) schema() []string {
	// SQLite has no float type name beyond REAL; Postgres wants DOUBLE
	// PRECISION. These are the only columns that differ.
	number := "DOUBLE PRECISION"
	if l.dialect == SQLite {
		number = "REAL"
	}

	return []string{
		// One row per event. No primary key: nothing looks an event up by
		// identity, they are only ever scanned by time and subject, and a
		// synthetic key would be an index to maintain on the hot write path
		// for nobody's benefit.
		`CREATE TABLE IF NOT EXISTS ` + l.table("cost_events") + ` (
	subject       TEXT NOT NULL,
	resource_type TEXT NOT NULL DEFAULT '',
	resource_id   TEXT NOT NULL DEFAULT '',
	provider      TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	units         ` + number + ` NOT NULL DEFAULT 0,
	unit_kind     TEXT NOT NULL DEFAULT '',
	cost_amount   ` + number + ` NOT NULL DEFAULT 0,
	occurred_ms   BIGINT NOT NULL DEFAULT 0,
	day_ms        BIGINT NOT NULL DEFAULT 0,
	labels        TEXT NOT NULL DEFAULT ''
)`,

		// Both reads of this table are bounded by time: the drill-down and the
		// retention sweep.
		`CREATE INDEX IF NOT EXISTS ` + l.table("cost_events_occurred_idx") +
			` ON ` + l.table("cost_events") + ` (occurred_ms)`,

		`CREATE INDEX IF NOT EXISTS ` + l.table("cost_events_subject_idx") +
			` ON ` + l.table("cost_events") + ` (subject, occurred_ms)`,

		// The rollup. Its primary key is every dimension a report groups on, so
		// the upsert in Record has something to conflict against and the read
		// path never has to deduplicate.
		//
		// labels is carried rather than keyed: a resource's containers can
		// change, and keying on them would split one twin's day into a row
		// before the move and a row after — two rows that a report would then
		// show as two twins.
		`CREATE TABLE IF NOT EXISTS ` + l.table("cost_daily") + ` (
	day_ms        BIGINT NOT NULL,
	subject       TEXT NOT NULL,
	resource_type TEXT NOT NULL DEFAULT '',
	resource_id   TEXT NOT NULL DEFAULT '',
	provider      TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	unit_kind     TEXT NOT NULL DEFAULT '',
	units         ` + number + ` NOT NULL DEFAULT 0,
	cost_amount   ` + number + ` NOT NULL DEFAULT 0,
	events        BIGINT NOT NULL DEFAULT 0,
	labels        TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (day_ms, subject, resource_type, resource_id, provider, model, unit_kind)
)`,

		// Every report is a range scan over days.
		`CREATE INDEX IF NOT EXISTS ` + l.table("cost_daily_day_idx") +
			` ON ` + l.table("cost_daily") + ` (day_ms)`,
	}
}

// execer is the subset of *sql.Tx and *sql.Conn this package uses, so both
// transaction styles share one code path.
type execer interface {
	ExecContext(context.Context, string, ...any) (stdsql.Result, error)
	QueryContext(context.Context, string, ...any) (*stdsql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *stdsql.Row
}

// withWrite runs fn inside a transaction holding a write lock for its duration,
// which is what keeps an event and its rollup increment atomic.
//
// The dialect handling is quota/sql's, for the reasons that package documents:
// SQLite needs BEGIN IMMEDIATE issued on a raw connection, and this process's
// writers queue on a mutex rather than in SQLite's busy handler.
func (l *Ledger) withWrite(ctx context.Context, fn func(execer) error) error {
	if l.dialect != SQLite {
		tx, err := l.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	l.writes.Lock()
	defer l.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	conn, err := l.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	if err := fn(conn); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
