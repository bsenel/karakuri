// Package sql provides a database/sql-backed implementation of cost.Ledger.
//
// It takes no ORM and no driver dependency: the caller opens its own *sql.DB
// and this package speaks plain SQL against it, in the same shape quota/sql
// does.
//
// # Raw events and a rollup
//
// Two tables, written together. cost_events holds one row per event, which is
// what answers "which objective spent this"; cost_daily holds one row per
// (day, subject, resource, provider, model), which is what a thirty-day report
// reads. A report over raw rows works and gets slower every week, and a
// deployment that only ever kept the rollup can never answer the drill-down
// question — so both, and the rollup is maintained on the write path rather
// than by a background job.
//
// That last choice is the load-bearing one. A background aggregator needs a
// scheduler, a watermark, and an answer for what a report shows while it is
// behind; folding the increment into the same transaction as the insert costs
// one extra upsert per event and means the two can never disagree.
//
// Raw events are prunable and the rollup is not. Sweep drops event rows past a
// retention horizon, and the daily totals survive — the shape most deployments
// want, where last year is a number and last week is a list.
package sql

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Dialect selects the SQL flavour.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// ErrUnsupportedDialect is returned by New for an unrecognised dialect.
var ErrUnsupportedDialect = errors.New("quota/cost/sql: unsupported dialect")

// Options configures a Ledger.
type Options struct {
	// Dialect defaults to SQLite.
	Dialect Dialect

	// TablePrefix namespaces the tables, e.g. "karakuri_" yields
	// "karakuri_cost_events".
	TablePrefix string
}

// Ledger implements cost.Ledger over database/sql.
type Ledger struct {
	db      *stdsql.DB
	dialect Dialect
	prefix  string

	// writes serialises this process's SQLite write transactions, for the
	// reason quota/sql documents at length: SQLite permits one writer, and
	// queueing here rather than in its busy handler is fair and respects
	// context cancellation. Unused on Postgres.
	writes sync.Mutex
}

var _ cost.Ledger = (*Ledger)(nil)

// New wraps an open database handle. The caller keeps ownership of db.
func New(db *stdsql.DB, opts Options) (*Ledger, error) {
	if db == nil {
		return nil, errors.New("quota/cost/sql: nil *sql.DB")
	}
	dialect := opts.Dialect
	if dialect == "" {
		dialect = SQLite
	}
	if dialect != SQLite && dialect != Postgres {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
	return &Ledger{db: db, dialect: dialect, prefix: opts.TablePrefix}, nil
}

func (l *Ledger) table(name string) string { return l.prefix + name }

func (l *Ledger) rebind(query string) string {
	if l.dialect != Postgres {
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

func (l *Ledger) excluded(column string) string {
	if l.dialect == Postgres {
		return "EXCLUDED." + column
	}
	return "excluded." + column
}

// Record writes the event and folds it into the day's rollup, in one
// transaction. Either both land or neither does — a rollup that drifted from
// its events would be a report that quietly disagrees with the drill-down
// behind it.
func (l *Ledger) Record(ctx context.Context, e cost.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	day := cost.Day(e.OccurredAt)
	labels := strings.Join(e.Labels, "\n")

	return l.withWrite(ctx, func(tx execer) error {
		if _, err := tx.ExecContext(ctx, l.rebind(
			`INSERT INTO `+l.table("cost_events")+`
			        (subject, resource_type, resource_id, provider, model,
			         units, unit_kind, cost_amount, occurred_ms, day_ms, labels)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			string(e.Subject), e.ResourceType, e.ResourceID, e.Provider, e.Model,
			e.Units, e.UnitKind, e.Cost, e.OccurredAt.UTC().UnixMilli(),
			day.UnixMilli(), labels); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, l.rebind(
			`INSERT INTO `+l.table("cost_daily")+`
			        (day_ms, subject, resource_type, resource_id, provider, model,
			         unit_kind, units, cost_amount, events, labels)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
			 ON CONFLICT (day_ms, subject, resource_type, resource_id, provider, model, unit_kind)
			 DO UPDATE SET
			        units = `+l.table("cost_daily")+`.units + `+l.excluded("units")+`,
			        cost_amount = `+l.table("cost_daily")+`.cost_amount + `+l.excluded("cost_amount")+`,
			        events = `+l.table("cost_daily")+`.events + 1,
			        labels = `+l.excluded("labels")),
			day.UnixMilli(), string(e.Subject), e.ResourceType, e.ResourceID,
			e.Provider, e.Model, e.UnitKind, e.Units, e.Cost, labels)
		return err
	})
}

// Aggregate answers a query from the rollup.
//
// The rollup rather than the events, deliberately: a report is the thing that
// has to stay fast as history accumulates, and it is also the thing that must
// keep working after raw events are pruned. Everything a report groups on is a
// column of the rollup — the drill-down to individual events is a different
// question, and Events answers it.
func (l *Ledger) Aggregate(ctx context.Context, q cost.Query) ([]cost.Bucket, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	rows, err := l.rollupRows(ctx, q)
	if err != nil {
		return nil, err
	}
	// Folding in Go rather than in SQL is what keeps this honest. The bucket
	// keys, the label expansion and the ordering are then literally the same
	// code the in-memory ledger runs, so the two cannot answer differently —
	// which is the whole reason cost.Fold is exported.
	return cost.Fold(rows, q), nil
}

// rollupRows reads the daily rows a query covers, as events one per row so Fold
// can treat them uniformly. The narrowing that matters — the time range, the
// subject and label filters — happens in SQL; only the grouping is in Go.
func (l *Ledger) rollupRows(ctx context.Context, q cost.Query) ([]cost.Event, error) {
	query := `SELECT day_ms, subject, resource_type, resource_id, provider, model,
	                 unit_kind, units, cost_amount, events, labels
	            FROM ` + l.table("cost_daily") + ` WHERE 1 = 1`
	var args []any

	if !q.Since.IsZero() {
		// The rollup is keyed on the day, so a range starting mid-afternoon
		// includes that whole day. Sharpening it would need the raw events, and
		// a report asking for "the last 30 days" means calendar days.
		query += ` AND day_ms >= ?`
		args = append(args, cost.Day(q.Since).UnixMilli())
	}
	if !q.Until.IsZero() {
		query += ` AND day_ms < ?`
		args = append(args, cost.Day(q.Until).UnixMilli())
	}
	if len(q.Subjects) > 0 {
		query += ` AND subject IN (` + placeholders(len(q.Subjects)) + `)`
		for _, s := range q.Subjects {
			args = append(args, string(s))
		}
	}
	if len(q.Providers) > 0 {
		query += ` AND provider IN (` + placeholders(len(q.Providers)) + `)`
		for _, p := range q.Providers {
			args = append(args, p)
		}
	}

	rows, err := l.db.QueryContext(ctx, l.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []cost.Event
	for rows.Next() {
		var (
			e      cost.Event
			dayMS  int64
			subj   string
			labels string
			events int
		)
		if err := rows.Scan(&dayMS, &subj, &e.ResourceType, &e.ResourceID,
			&e.Provider, &e.Model, &e.UnitKind, &e.Units, &e.Cost, &events, &labels); err != nil {
			return nil, err
		}
		e.Subject = quota.Key(subj)
		e.OccurredAt = time.UnixMilli(dayMS).UTC()
		e.Labels = splitLabels(labels)

		// Labels are filtered here rather than in SQL. Matching "any of these
		// labels" against a set stored in one column means LIKE with escaping,
		// which is both slower and easier to get subtly wrong than a comparison
		// over a handful of strings — and the row count is already bounded by
		// the range and subject filters above.
		if len(q.Labels) > 0 && !anyLabel(q.Labels, e.Labels) {
			continue
		}
		// Fold counts one event per row, so a rolled-up row carrying twelve
		// events has to say so.
		for range max(events, 1) {
			out = append(out, e)
		}
		// Only the first copy carries the units and cost; the rest are there so
		// Bucket.Events is right.
		for i := len(out) - max(events, 1) + 1; i < len(out); i++ {
			out[i].Units, out[i].Cost = 0, 0
		}
	}
	return out, rows.Err()
}

// Events returns the raw events behind a report, newest first — the drill-down
// from a total to the calls that produced it.
//
// It reads the event table rather than the rollup, so it answers nothing older
// than the retention horizon Sweep enforces.
func (l *Ledger) Events(ctx context.Context, q cost.Query) ([]cost.Event, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	query := `SELECT subject, resource_type, resource_id, provider, model,
	                 units, unit_kind, cost_amount, occurred_ms, labels
	            FROM ` + l.table("cost_events") + ` WHERE 1 = 1`
	var args []any

	if !q.Since.IsZero() {
		query += ` AND occurred_ms >= ?`
		args = append(args, q.Since.UTC().UnixMilli())
	}
	if !q.Until.IsZero() {
		query += ` AND occurred_ms < ?`
		args = append(args, q.Until.UTC().UnixMilli())
	}
	if len(q.Subjects) > 0 {
		query += ` AND subject IN (` + placeholders(len(q.Subjects)) + `)`
		for _, s := range q.Subjects {
			args = append(args, string(s))
		}
	}
	if len(q.Providers) > 0 {
		query += ` AND provider IN (` + placeholders(len(q.Providers)) + `)`
		for _, p := range q.Providers {
			args = append(args, p)
		}
	}
	query += ` ORDER BY occurred_ms DESC`
	if q.Limit > 0 {
		query += ` LIMIT ` + strconv.Itoa(q.Limit)
	}

	rows, err := l.db.QueryContext(ctx, l.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []cost.Event
	for rows.Next() {
		var (
			e          cost.Event
			subj       string
			occurredMS int64
			labels     string
		)
		if err := rows.Scan(&subj, &e.ResourceType, &e.ResourceID, &e.Provider,
			&e.Model, &e.Units, &e.UnitKind, &e.Cost, &occurredMS, &labels); err != nil {
			return nil, err
		}
		e.Subject = quota.Key(subj)
		e.OccurredAt = time.UnixMilli(occurredMS).UTC()
		e.Labels = splitLabels(labels)
		if len(q.Labels) > 0 && !anyLabel(q.Labels, e.Labels) {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Sweep deletes raw events older than the horizon and reports how many went.
//
// The rollup is untouched, which is the point: a deployment keeps the ability
// to say what last year cost without keeping every call that made it up.
func (l *Ledger) Sweep(ctx context.Context, before time.Time) (int64, error) {
	var affected int64
	err := l.withWrite(ctx, func(tx execer) error {
		res, err := tx.ExecContext(ctx, l.rebind(
			`DELETE FROM `+l.table("cost_events")+` WHERE occurred_ms < ?`),
			before.UTC().UnixMilli())
		if err != nil {
			return err
		}
		affected, _ = res.RowsAffected()
		return nil
	})
	return affected, err
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Labels are stored newline-joined in one column. A join table would be the
// textbook answer and would cost a join on every read of a set that is almost
// always two entries long — and the filtering that matters happens over the
// decoded slice anyway.
func splitLabels(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func anyLabel(want, have []string) bool {
	for _, w := range want {
		for _, h := range have {
			if w == h {
				return true
			}
		}
	}
	return false
}
