// Package sql provides a database/sql-backed implementation of quota.Backend.
//
// It takes no ORM and no driver dependency: the caller opens its own *sql.DB
// with whatever driver it already uses, and this package speaks plain SQL
// against it. Only two dialects differ in ways that matter — placeholder
// syntax, one column type, and how a write transaction is opened — and all
// three are handled by the Dialect switch rather than by separate
// implementations.
//
// # What this backend is for
//
// Counters that outlive a process and are shared across replicas: daily and
// monthly quotas, per-capability caps, anything an operator would expect to
// survive a restart. It is a poor fit for a high-frequency per-request rate
// limit, where a round trip and a row lock per request is a lot of database for
// very little arithmetic — reach for quota/valkey or the in-memory backend
// there.
//
// [quota.SlidingLog] in particular stores one row per consumption. That is fine
// for "a thousand a day" and wrong for "sixty a minute".
//
// # SQLite callers must set a busy timeout
//
// Take opens its transaction with BEGIN IMMEDIATE, which takes the write lock
// up front. Without a busy timeout SQLite returns SQLITE_BUSY the instant a
// second connection tries, so under any concurrency most takes fail rather than
// wait — and a limiter whose errors are ignored is a limiter that is not
// limiting. Open the database with one:
//
//	sql.Open("sqlite", "file:quota.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
//
// Postgres needs nothing equivalent; its transactions block on the row lock.
package sql

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// Dialect selects the SQL flavour. Anything else is rejected at construction
// rather than producing statements that fail at the first query.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// ErrUnsupportedDialect is returned by New for an unrecognised dialect.
var ErrUnsupportedDialect = errors.New("quota/sql: unsupported dialect")

// Options configures a Backend.
type Options struct {
	// Dialect defaults to SQLite.
	Dialect Dialect

	// TablePrefix namespaces the tables, e.g. "karakuri_" yields
	// "karakuri_quota_counters". Defaults to none.
	TablePrefix string
}

// Backend implements quota.Backend over database/sql.
type Backend struct {
	db      *stdsql.DB
	dialect Dialect
	prefix  string
}

var _ quota.Backend = (*Backend)(nil)

// New wraps an open database handle. The caller keeps ownership of db — Backend
// never closes it.
func New(db *stdsql.DB, opts Options) (*Backend, error) {
	if db == nil {
		return nil, errors.New("quota/sql: nil *sql.DB")
	}
	dialect := opts.Dialect
	if dialect == "" {
		dialect = SQLite
	}
	if dialect != SQLite && dialect != Postgres {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
	return &Backend{db: db, dialect: dialect, prefix: opts.TablePrefix}, nil
}

func (b *Backend) table(name string) string { return b.prefix + name }

// rebind converts "?" placeholders to the dialect's own form. This is the only
// dialect-specific code on the query path besides the lock clause.
func (b *Backend) rebind(query string) string {
	if b.dialect != Postgres {
		return query
	}
	var out strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			out.WriteByte('$')
			out.WriteString(strconv.Itoa(n))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// lockClause is what serialises concurrent takes on Postgres. SQLite gets it
// from BEGIN IMMEDIATE instead — see withWrite.
func (b *Backend) lockClause() string {
	if b.dialect == Postgres {
		return " FOR UPDATE"
	}
	return ""
}

// execer is the subset of *sql.Tx and *sql.Conn this package uses, so the two
// transaction styles below share one code path.
type execer interface {
	ExecContext(context.Context, string, ...any) (stdsql.Result, error)
	QueryContext(context.Context, string, ...any) (*stdsql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *stdsql.Row
}

// withWrite runs fn inside a transaction that holds a write lock for its whole
// duration, which is what makes the read-modify-write in Take atomic per key —
// the property quota.Backend requires and the shared contract suite proves.
//
// The two dialects need different things:
//
//   - Postgres: an ordinary transaction, with the row locked by FOR UPDATE.
//   - SQLite: BEGIN IMMEDIATE. database/sql's BeginTx issues a *deferred*
//     BEGIN, which SQLite upgrades to a write lock only at the first write —
//     and can then fail with SQLITE_BUSY after we have already read, or worse,
//     let two readers compute from the same state. IMMEDIATE takes the write
//     lock up front. It has to be issued on a raw *sql.Conn because database/sql
//     owns the BEGIN inside a Tx.
func (b *Backend) withWrite(ctx context.Context, fn func(execer) error) error {
	if b.dialect != SQLite {
		tx, err := b.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	conn, err := b.db.Conn(ctx)
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

func (b *Backend) Take(ctx context.Context, key quota.Key, p quota.Policy, n int, now time.Time) (quota.Decision, error) {
	if err := p.Validate(); err != nil {
		return quota.Decision{}, err
	}
	var d quota.Decision
	err := b.withWrite(ctx, func(x execer) error {
		// Materialise the row before locking it. FOR UPDATE on a row that does
		// not exist locks nothing, so without this two racers on a fresh key
		// would both read "empty" and both be allowed.
		if _, err := x.ExecContext(ctx, b.rebind(
			`INSERT INTO `+b.table("quota_counters")+` (key) VALUES (?) ON CONFLICT (key) DO NOTHING`,
		), string(key)); err != nil {
			return err
		}

		st, err := b.loadState(ctx, x, key, p, now, true)
		if err != nil {
			return err
		}
		d, err = quota.Apply(&st, p, n, now)
		if err != nil {
			return err
		}
		return b.saveState(ctx, x, key, p, st, n, d, now)
	})
	if err != nil {
		return quota.Decision{}, err
	}
	return d, nil
}

func (b *Backend) Peek(ctx context.Context, key quota.Key, p quota.Policy, now time.Time) (quota.Decision, error) {
	if err := p.Validate(); err != nil {
		return quota.Decision{}, err
	}
	// No transaction and no row created: a usage endpoint must not spend the
	// budget it reports on, and must not mint a counter for every key it is
	// asked about.
	st, err := b.loadState(ctx, b.db, key, p, now, false)
	if err != nil {
		return quota.Decision{}, err
	}
	d, err := quota.Apply(&st, p, 1, now)
	if err != nil {
		return quota.Decision{}, err
	}
	if d.Allowed {
		// The unit came off a struct that is about to be discarded.
		d.Remaining++
	}
	return d, nil
}

func (b *Backend) Reset(ctx context.Context, key quota.Key) error {
	return b.withWrite(ctx, func(x execer) error {
		for _, table := range []string{"quota_counters", "quota_events"} {
			if _, err := x.ExecContext(ctx,
				b.rebind(`DELETE FROM `+b.table(table)+` WHERE key = ?`), string(key),
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// Sweep deletes state that has outlived its policy window. Nothing calls it
// automatically: a limiter should not decide on its own to run a bulk delete
// against somebody's production database. Call it from a housekeeping job.
func (b *Backend) Sweep(ctx context.Context, now time.Time) (int64, error) {
	var deleted int64
	err := b.withWrite(ctx, func(x execer) error {
		res, err := x.ExecContext(ctx, b.rebind(
			`DELETE FROM `+b.table("quota_counters")+` WHERE expires_ms > 0 AND expires_ms < ?`,
		), ms(now))
		if err != nil {
			return err
		}
		deleted, _ = res.RowsAffected()

		// Events belong to keys whose counter row is gone.
		_, err = x.ExecContext(ctx,
			`DELETE FROM `+b.table("quota_events")+` WHERE key NOT IN (SELECT key FROM `+b.table("quota_counters")+`)`)
		return err
	})
	return deleted, err
}

// loadState reads the stored state for key. lock is honoured on Postgres only;
// on SQLite the write lock is already held by the surrounding transaction.
func (b *Backend) loadState(
	ctx context.Context, x execer, key quota.Key, p quota.Policy, now time.Time, lock bool,
) (quota.State, error) {
	clause := ""
	if lock {
		clause = b.lockClause()
	}

	var (
		st                               quota.State
		algorithm                        string
		tokens                           float64
		lastMS, windowStartMS, usedUnits int64
	)
	err := x.QueryRowContext(ctx, b.rebind(
		`SELECT algorithm, tokens, last_ms, window_start_ms, used_units FROM `+
			b.table("quota_counters")+` WHERE key = ?`+clause,
	), string(key)).Scan(&algorithm, &tokens, &lastMS, &windowStartMS, &usedUnits)
	switch {
	case errors.Is(err, stdsql.ErrNoRows):
		// A key nobody has taken against yet. An empty state is the answer, not
		// an error.
		return quota.State{Algorithm: p.Algorithm}, nil
	case err != nil:
		return quota.State{}, err
	}

	st = quota.State{
		Algorithm:   quota.Algorithm(algorithm),
		Tokens:      tokens,
		Last:        fromMS(lastMS),
		WindowStart: fromMS(windowStartMS),
		Count:       int(usedUnits),
	}
	if p.Algorithm != quota.SlidingLog {
		return st, nil
	}

	// Only entries still inside the window are worth loading — the row count is
	// bounded by the limit rather than by how long the key has existed.
	rows, err := x.QueryContext(ctx, b.rebind(
		`SELECT at_ms, n FROM `+b.table("quota_events")+
			` WHERE key = ? AND at_ms > ? ORDER BY at_ms`,
	), string(key), ms(now.Add(-p.Window)))
	if err != nil {
		return quota.State{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var atMS, n int64
		if err := rows.Scan(&atMS, &n); err != nil {
			return quota.State{}, err
		}
		st.Log = append(st.Log, quota.Event{At: fromMS(atMS), N: int(n)})
	}
	return st, rows.Err()
}

// saveState writes back what Apply left behind.
func (b *Backend) saveState(
	ctx context.Context, x execer, key quota.Key, p quota.Policy,
	st quota.State, n int, d quota.Decision, now time.Time,
) error {
	// Two windows past the last use: one so an in-flight window is never
	// dropped mid-flight, one as slack for a replica whose clock runs behind.
	expires := ms(now.Add(2 * p.Window))

	if _, err := x.ExecContext(ctx, b.rebind(
		`UPDATE `+b.table("quota_counters")+
			` SET algorithm = ?, tokens = ?, last_ms = ?, window_start_ms = ?, used_units = ?, expires_ms = ?`+
			` WHERE key = ?`,
	), string(st.Algorithm), st.Tokens, ms(st.Last), ms(st.WindowStart), int64(st.Count), expires,
		string(key)); err != nil {
		return err
	}

	if p.Algorithm != quota.SlidingLog {
		return nil
	}

	// Apply trimmed the in-memory log; mirror that in the table so it does not
	// grow without bound.
	if _, err := x.ExecContext(ctx, b.rebind(
		`DELETE FROM `+b.table("quota_events")+` WHERE key = ? AND at_ms <= ?`,
	), string(key), ms(now.Add(-p.Window))); err != nil {
		return err
	}

	// Apply appends exactly one event when it allows a costed take, so this is
	// the whole of the delta rather than something that has to be diffed.
	if d.Allowed && n > 0 {
		if _, err := x.ExecContext(ctx, b.rebind(
			`INSERT INTO `+b.table("quota_events")+` (key, at_ms, n) VALUES (?, ?, ?)`,
		), string(key), ms(now), int64(n)); err != nil {
			return err
		}
	}
	return nil
}

// Timestamps are stored as epoch milliseconds. Drivers disagree about how
// DATETIME round-trips — time zones, precision, and whether you get a string
// back — and an integer disagrees with nobody.
func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}

func fromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}
