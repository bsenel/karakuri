package sql

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// Overrides and requests, persisted.
//
// They live on the same Backend as the counters because they are the same
// deployment's state and a caller already has this handle — but they are not
// counters, and the difference shows in how they are written. A counter is
// read-modify-written under a lock on the hot path; an override is written when
// a human approves something and read constantly, which is what
// quota.Resolver's cache exists to exploit. Neither of these needs withWrite's
// serialisation, so neither takes it.

var _ quota.OverrideStore = (*Backend)(nil)
var _ quota.RequestStore = (*Backend)(nil)

// Overrides returns every override for a subject, expired ones included —
// filtering by time is the resolver's job.
func (b *Backend) Overrides(ctx context.Context, subject quota.Key) ([]quota.Override, error) {
	return b.queryOverrides(ctx,
		`SELECT subject, name, cap_units, window_ms, expires_ms, reason
		   FROM `+b.table("quota_overrides")+`
		  WHERE subject = ?
		  ORDER BY name`, string(subject))
}

// ListOverrides returns every override in the store.
func (b *Backend) ListOverrides(ctx context.Context) ([]quota.Override, error) {
	return b.queryOverrides(ctx,
		`SELECT subject, name, cap_units, window_ms, expires_ms, reason
		   FROM `+b.table("quota_overrides")+`
		  ORDER BY subject, name`)
}

func (b *Backend) queryOverrides(ctx context.Context, query string, args ...any) ([]quota.Override, error) {
	rows, err := b.db.QueryContext(ctx, b.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []quota.Override
	for rows.Next() {
		var (
			o         quota.Override
			subject   string
			windowMS  int64
			expiresMS int64
		)
		if err := rows.Scan(&subject, &o.Name, &o.Cap, &windowMS, &expiresMS, &o.Reason); err != nil {
			return nil, err
		}
		o.Subject = quota.Key(subject)
		o.Window = time.Duration(windowMS) * time.Millisecond
		o.ExpiresAt = fromMS(expiresMS)
		out = append(out, o)
	}
	return out, rows.Err()
}

// PutOverride stores one, replacing any with the same subject and name.
func (b *Backend) PutOverride(ctx context.Context, o quota.Override) error {
	if err := o.Validate(); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx, b.rebind(
		`INSERT INTO `+b.table("quota_overrides")+`
		        (subject, name, cap_units, window_ms, expires_ms, reason)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (subject, name) DO UPDATE SET
		        cap_units = `+b.excluded("cap_units")+`,
		        window_ms = `+b.excluded("window_ms")+`,
		        expires_ms = `+b.excluded("expires_ms")+`,
		        reason = `+b.excluded("reason")),
		string(o.Subject), o.Name, o.Cap, o.Window.Milliseconds(), ms(o.ExpiresAt), o.Reason)
	return err
}

// DeleteOverride removes one. Removing what is not there is not an error, so a
// retried revocation is safe.
func (b *Backend) DeleteOverride(ctx context.Context, subject quota.Key, name string) error {
	_, err := b.db.ExecContext(ctx, b.rebind(
		`DELETE FROM `+b.table("quota_overrides")+` WHERE subject = ? AND name = ?`),
		string(subject), name)
	return err
}

// PutRequest stores a request, replacing any with the same ID.
func (b *Backend) PutRequest(ctx context.Context, r quota.Request) error {
	_, err := b.db.ExecContext(ctx, b.rebind(
		`INSERT INTO `+b.table("quota_requests")+`
		        (id, subject, name, cap_units, window_ms, expires_ms, reason,
		         status, requested_by, created_ms, decided_by, decided_ms, decision_note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET
		        subject = `+b.excluded("subject")+`,
		        name = `+b.excluded("name")+`,
		        cap_units = `+b.excluded("cap_units")+`,
		        window_ms = `+b.excluded("window_ms")+`,
		        expires_ms = `+b.excluded("expires_ms")+`,
		        reason = `+b.excluded("reason")+`,
		        status = `+b.excluded("status")+`,
		        requested_by = `+b.excluded("requested_by")+`,
		        created_ms = `+b.excluded("created_ms")+`,
		        decided_by = `+b.excluded("decided_by")+`,
		        decided_ms = `+b.excluded("decided_ms")+`,
		        decision_note = `+b.excluded("decision_note")),
		r.ID, string(r.Subject), r.Name, r.Cap, r.Window.Milliseconds(),
		ms(r.ExpiresAt), r.Reason, string(r.Status), r.RequestedBy,
		ms(r.CreatedAt), r.DecidedBy, ms(r.DecidedAt), r.DecisionNote)
	return err
}

// GetRequest returns one request, or quota.ErrRequestNotFound.
func (b *Backend) GetRequest(ctx context.Context, id string) (quota.Request, error) {
	rows, err := b.queryRequests(ctx,
		b.requestSelect()+` WHERE id = ?`, id)
	if err != nil {
		return quota.Request{}, err
	}
	if len(rows) == 0 {
		return quota.Request{}, fmt.Errorf("%w: %q", quota.ErrRequestNotFound, id)
	}
	return rows[0], nil
}

// ListRequests returns matching requests, newest first.
func (b *Backend) ListRequests(ctx context.Context, f quota.RequestFilter) ([]quota.Request, error) {
	query := b.requestSelect() + ` WHERE 1 = 1`
	var args []any
	if f.Subject != "" {
		query += ` AND subject = ?`
		args = append(args, string(f.Subject))
	}
	if f.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(f.Status))
	}
	if f.RequestedBy != "" {
		query += ` AND requested_by = ?`
		args = append(args, f.RequestedBy)
	}
	// ID breaks ties so two requests submitted in the same millisecond still
	// page in a stable order.
	query += ` ORDER BY created_ms DESC, id ASC`
	return b.queryRequests(ctx, query, args...)
}

func (b *Backend) requestSelect() string {
	return `SELECT id, subject, name, cap_units, window_ms, expires_ms, reason,
	               status, requested_by, created_ms, decided_by, decided_ms, decision_note
	          FROM ` + b.table("quota_requests")
}

func (b *Backend) queryRequests(ctx context.Context, query string, args ...any) ([]quota.Request, error) {
	rows, err := b.db.QueryContext(ctx, b.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []quota.Request
	for rows.Next() {
		var (
			r         quota.Request
			subject   string
			status    string
			windowMS  int64
			expiresMS int64
			createdMS int64
			decidedMS int64
		)
		if err := rows.Scan(&r.ID, &subject, &r.Name, &r.Cap, &windowMS, &expiresMS,
			&r.Reason, &status, &r.RequestedBy, &createdMS,
			&r.DecidedBy, &decidedMS, &r.DecisionNote); err != nil {
			return nil, err
		}
		r.Subject, r.Status = quota.Key(subject), quota.RequestStatus(status)
		r.Window = time.Duration(windowMS) * time.Millisecond
		r.ExpiresAt, r.CreatedAt, r.DecidedAt = fromMS(expiresMS), fromMS(createdMS), fromMS(decidedMS)
		out = append(out, r)
	}
	return out, rows.Err()
}

// excluded names the proposed row in an upsert. The two dialects spell it
// differently and this is the only place it matters.
func (b *Backend) excluded(column string) string {
	if b.dialect == Postgres {
		return "EXCLUDED." + column
	}
	return "excluded." + column
}
