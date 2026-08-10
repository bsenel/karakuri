package quota

import (
	"context"
	"fmt"
	"time"
)

// Period is the calendar span a Quota resets on.
type Period string

const (
	Hourly  Period = "hourly"
	Daily   Period = "daily"
	Monthly Period = "monthly"
)

// Quota is a hard cap over a calendar period — "1000 writes a day", "50 dollars
// a month" — as opposed to a Policy, which smooths traffic over seconds and
// minutes.
//
// The distinction is not cosmetic. A rate limit refuses a request and expects
// it back in a moment; a quota refuses it until tomorrow. They want different
// messages, different alerting, and usually different responses from the caller.
type Quota struct {
	// Name distinguishes one quota from another on the same subject, and forms
	// part of the storage key. Required.
	Name string

	// Cap is the number of units permitted per period.
	Cap int

	Period Period
}

// Validate reports whether the quota is usable. Call it at startup.
func (q Quota) Validate() error {
	if q.Name == "" {
		return fmt.Errorf("%w: quota name is empty", ErrInvalidPolicy)
	}
	if q.Cap <= 0 {
		return fmt.Errorf("%w: quota %q cap must be positive, got %d", ErrInvalidPolicy, q.Name, q.Cap)
	}
	switch q.Period {
	case Hourly, Daily, Monthly:
	default:
		return fmt.Errorf("%w: quota %q has unknown period %q", ErrInvalidPolicy, q.Name, q.Period)
	}
	return nil
}

// Key returns the storage key for subject in the period containing now.
//
// The period is *in the key*: "twin:t1|daily-writes|2026-08-10". That is what
// makes the reset exact and identical on every backend — at midnight the key
// changes, so the new period starts at zero without anything having to notice
// the boundary, and the old key simply ages out. No backend has to implement a
// calendar.
func (q Quota) Key(subject Key, now time.Time) Key {
	return JoinKey(string(subject), q.Name, q.bucket(now))
}

func (q Quota) bucket(now time.Time) string {
	now = now.UTC()
	switch q.Period {
	case Hourly:
		return now.Format("2006-01-02T15")
	case Monthly:
		return now.Format("2006-01")
	default:
		return now.Format("2006-01-02")
	}
}

// PeriodEnd is when the current period rolls over, in UTC. Months are handled
// by calendar arithmetic rather than a nominal 30 days, so a quota does not
// drift a day and a half every year.
func (q Quota) PeriodEnd(now time.Time) time.Time {
	now = now.UTC()
	switch q.Period {
	case Hourly:
		return now.Truncate(time.Hour).Add(time.Hour)
	case Monthly:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	default:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	}
}

// policy renders the quota as a counter the backend can hold.
//
// FixedWindow with a window wide enough to outlive the period: the key already
// carries the period, so the window's only remaining job is to tell the backend
// how long the entry is worth keeping. Sizing it at two periods means a slow or
// skewed clock cannot expire a bucket that is still being written to.
func (q Quota) policy() Policy {
	var span time.Duration
	switch q.Period {
	case Hourly:
		span = time.Hour
	case Monthly:
		span = 31 * 24 * time.Hour
	default:
		span = 24 * time.Hour
	}
	return Policy{Algorithm: FixedWindow, Limit: q.Cap, Window: 2 * span}
}

// Take consumes n units of the quota for subject.
//
// The Decision's ResetAt and RetryAfter are corrected to the true period
// boundary — the backend only knows about its own window, which is deliberately
// wider.
func (q Quota) Take(ctx context.Context, b Backend, subject Key, n int, now time.Time) (Decision, error) {
	return q.decide(ctx, b, subject, n, now, true)
}

// Peek reports usage without consuming any.
func (q Quota) Peek(ctx context.Context, b Backend, subject Key, now time.Time) (Decision, error) {
	return q.decide(ctx, b, subject, 0, now, false)
}

// Reset clears the current period's usage for subject. It is the admin
// override: it affects the period containing now and nothing else, so resetting
// today cannot hand back yesterday's budget.
func (q Quota) Reset(ctx context.Context, b Backend, subject Key, now time.Time) error {
	if err := q.Validate(); err != nil {
		return err
	}
	return b.Reset(ctx, q.Key(subject, now))
}

func (q Quota) decide(ctx context.Context, b Backend, subject Key, n int, now time.Time, commit bool) (Decision, error) {
	if err := q.Validate(); err != nil {
		return Decision{}, err
	}
	key, p := q.Key(subject, now), q.policy()

	var (
		d   Decision
		err error
	)
	if commit {
		d, err = b.Take(ctx, key, p, n, now)
	} else {
		d, err = b.Peek(ctx, key, p, now)
	}
	if err != nil {
		return Decision{}, err
	}

	end := q.PeriodEnd(now)
	d.ResetAt = end
	if !d.Allowed {
		d.RetryAfter = end.Sub(now)
	}
	return d, nil
}
