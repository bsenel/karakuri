package quota

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestDecisionUsed(t *testing.T) {
	tests := []struct {
		name string
		d    Decision
		want float64
	}{
		{"untouched", Decision{Limit: 10, Remaining: 10}, 0},
		{"half", Decision{Limit: 10, Remaining: 5}, 0.5},
		{"pressure threshold", Decision{Limit: 10, Remaining: 2}, 0.8},
		{"exhausted", Decision{Limit: 10, Remaining: 0}, 1},
		// A limit of zero admits nothing, so it reads as fully used rather
		// than dividing by zero.
		{"zero limit", Decision{Limit: 0, Remaining: 0}, 1},
		{"negative limit", Decision{Limit: -5, Remaining: 0}, 1},
		// Defensive: a backend reporting more remaining than the limit should
		// not produce a negative fraction that slips under every threshold.
		{"remaining above limit", Decision{Limit: 10, Remaining: 15}, 0},
		{"remaining below zero", Decision{Limit: 10, Remaining: -5}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.Used(); got != tc.want {
				t.Errorf("Used() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecisionNormalize(t *testing.T) {
	// A refusal with no wait tells a client to retry immediately, which turns
	// a limiter into a busy loop.
	got := Decision{Allowed: false, RetryAfter: 0}.Normalize()
	if got.RetryAfter <= 0 {
		t.Errorf("refusal normalised to retry-after %s, want a positive wait", got.RetryAfter)
	}
	if got := (Decision{Allowed: true, RetryAfter: time.Minute}).Normalize(); got.RetryAfter != 0 {
		t.Errorf("allowed decision kept retry-after %s, want 0", got.RetryAfter)
	}
	if got := (Decision{Remaining: -3}).Normalize(); got.Remaining != 0 {
		t.Errorf("remaining normalised to %d, want 0", got.Remaining)
	}
	if got := (Decision{Allowed: false, RetryAfter: time.Hour}).Normalize(); got.RetryAfter != time.Hour {
		t.Errorf("a real wait was overwritten: %s", got.RetryAfter)
	}
}

func TestDecisionSetHeaders(t *testing.T) {
	reset := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

	t.Run("allowed", func(t *testing.T) {
		h := http.Header{}
		Decision{Allowed: true, Limit: 60, Remaining: 42, ResetAt: reset}.SetHeaders(h)
		want := map[string]string{
			"X-Ratelimit-Limit":     "60",
			"X-Ratelimit-Remaining": "42",
			"X-Ratelimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
		}
		for k, v := range want {
			if got := h.Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		// Retry-After on a successful response would be nonsense.
		if got := h.Get("Retry-After"); got != "" {
			t.Errorf("Retry-After = %q on an allowed request", got)
		}
	})

	t.Run("refused rounds the wait up", func(t *testing.T) {
		h := http.Header{}
		Decision{Limit: 60, Remaining: 0, RetryAfter: 1500 * time.Millisecond, ResetAt: reset}.SetHeaders(h)
		// 1.5s must not round down to 1: a client that retries early is
		// refused again, which is the loop the header exists to prevent.
		if got := h.Get("Retry-After"); got != "2" {
			t.Errorf("Retry-After = %q, want 2", got)
		}
	})

	t.Run("sub-second waits floor at one", func(t *testing.T) {
		h := http.Header{}
		Decision{Limit: 1, RetryAfter: time.Millisecond}.SetHeaders(h)
		if got := h.Get("Retry-After"); got != "1" {
			t.Errorf("Retry-After = %q, want 1 — zero invites an instant retry", got)
		}
	})

	t.Run("negative remaining is not published", func(t *testing.T) {
		h := http.Header{}
		Decision{Allowed: true, Limit: 10, Remaining: -4}.SetHeaders(h)
		if got := h.Get("X-RateLimit-Remaining"); got != "0" {
			t.Errorf("X-RateLimit-Remaining = %q, want 0", got)
		}
		// A zero ResetAt is not a real instant; publishing it as a 1970
		// timestamp would be worse than omitting the header.
		if got := h.Get("X-RateLimit-Reset"); got != "" {
			t.Errorf("X-RateLimit-Reset = %q with no reset time known", got)
		}
	})
}
