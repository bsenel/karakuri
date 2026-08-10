package quota

import (
	"net/http"
	"strconv"
	"time"
)

// Decision is a backend's answer for one key: whether the request may proceed,
// and everything a caller needs to tell the client why not.
//
// It is deliberately complete. A backend already knows the remaining budget and
// when it refills — recomputing either in the middleware would mean modelling
// the algorithm twice, in two places that could disagree.
type Decision struct {
	// Allowed reports whether the requested cost was granted. When false,
	// nothing was consumed: a refused request does not deepen the hole it is
	// already in.
	Allowed bool

	// Limit is the policy ceiling, echoed so callers rendering headers do not
	// have to carry the Policy alongside the Decision.
	Limit int

	// Remaining is the budget left after this call, floored at zero.
	Remaining int

	// RetryAfter is how long until the request would succeed. Zero when
	// Allowed.
	RetryAfter time.Duration

	// ResetAt is when the budget returns to Limit — the end of the current
	// window, or when a bucket finishes refilling.
	ResetAt time.Time
}

// minRetryAfter is the floor on a refusal's wait. Zero would tell a client to
// retry immediately, which turns a limiter into a busy loop; the value only has
// to be positive, since SetHeaders rounds the header up to a whole second.
const minRetryAfter = time.Millisecond

// Normalize repairs a Decision so it satisfies the [Backend] contract.
//
// For backend implementors: call it on the way out of Take and Peek. It is
// exported because the SQL and Valkey backends are separate modules and would
// otherwise each re-derive the same two rules — a refusal always carries a
// positive wait, and Remaining never goes below zero.
func (d Decision) Normalize() Decision {
	if d.Remaining < 0 {
		d.Remaining = 0
	}
	if d.Allowed {
		d.RetryAfter = 0
	} else if d.RetryAfter < minRetryAfter {
		d.RetryAfter = minRetryAfter
	}
	return d
}

// Used is the fraction of the limit consumed, in [0,1].
//
// This is what a pressure threshold is measured against: "warn at 80%" means
// Used() >= 0.8. A zero limit reads as fully used rather than dividing by zero,
// because a limit of zero admits nothing.
func (d Decision) Used() float64 {
	if d.Limit <= 0 {
		return 1
	}
	used := float64(d.Limit-d.Remaining) / float64(d.Limit)
	if used < 0 {
		return 0
	}
	if used > 1 {
		return 1
	}
	return used
}

// SetHeaders writes the conventional rate-limit headers onto a response.
//
// The X-RateLimit-* trio is the de-facto set every HTTP client library already
// understands. Retry-After is only meaningful on a refusal and is written only
// then, in seconds — the integer form, because the HTTP-date form is worse for
// clients doing arithmetic and no better for humans.
//
// Rounds up: a client told to wait 0 seconds retries immediately and is refused
// again, which turns a limiter into a busy loop.
func (d Decision) SetHeaders(h http.Header) {
	h.Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(max(d.Remaining, 0)))
	if !d.ResetAt.IsZero() {
		h.Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))
	}
	if !d.Allowed {
		secs := int64(d.RetryAfter / time.Second)
		if d.RetryAfter%time.Second > 0 {
			secs++
		}
		h.Set("Retry-After", strconv.FormatInt(max(secs, 1), 10))
	}
}
