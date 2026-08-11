package quota

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPolicyValidate(t *testing.T) {
	tests := []struct {
		name string
		p    Policy
		want string // substring of the message; empty means valid
	}{
		{"token bucket", Policy{TokenBucket, 10, time.Minute, 0}, ""},
		{"fixed window", Policy{FixedWindow, 10, time.Minute, 0}, ""},
		{"sliding log", Policy{SlidingLog, 10, time.Minute, 0}, ""},
		{"explicit rate", Policy{TokenBucket, 10, time.Minute, 0.5}, ""},

		{"no algorithm", Policy{"", 10, time.Minute, 0}, "algorithm is empty"},
		{"unknown algorithm", Policy{"leaky", 10, time.Minute, 0}, "unknown algorithm"},
		{"zero limit", Policy{FixedWindow, 0, time.Minute, 0}, "limit must be positive"},
		{"negative limit", Policy{FixedWindow, -1, time.Minute, 0}, "limit must be positive"},
		{"zero window", Policy{FixedWindow, 10, 0, 0}, "window must be positive"},
		{"negative window", Policy{FixedWindow, 10, -time.Minute, 0}, "window must be positive"},
		{"negative rate", Policy{TokenBucket, 10, time.Minute, -1}, "rate must not be negative"},
		// Rate on a window algorithm is a misunderstanding, not a harmless
		// extra: it reads as if it does something.
		{"rate on a window algorithm", Policy{FixedWindow, 10, time.Minute, 1}, "rate applies to"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.want)
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Errorf("error does not wrap ErrInvalidPolicy: %v", err)
			}
			if got := err.Error(); !strings.Contains(got, tc.want) {
				t.Errorf("error = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

func TestPolicyRefillRate(t *testing.T) {
	// Derived from limit and window when unset...
	if got := (Policy{TokenBucket, 60, time.Minute, 0}).RatePerSecond(); got != 1 {
		t.Errorf("60/min derived rate = %v, want 1/s", got)
	}
	// ...and honoured when set, which is what decouples burst from throughput.
	if got := (Policy{TokenBucket, 60, time.Minute, 5}).RatePerSecond(); got != 5 {
		t.Errorf("explicit rate = %v, want 5", got)
	}
}

func TestPolicyHelpers(t *testing.T) {
	if got := PerMinute(60); got != (Policy{TokenBucket, 60, time.Minute, 0}) {
		t.Errorf("PerMinute(60) = %+v", got)
	}
	if got := PerSecond(5); got != (Policy{TokenBucket, 5, time.Second, 0}) {
		t.Errorf("PerSecond(5) = %+v", got)
	}

	// Burst raises the ceiling while pinning the throughput the original policy
	// implied — "60 a minute, but tolerate 20 at once" is one token a second.
	b := PerMinute(60).Burst(20)
	if b.Limit != 20 {
		t.Errorf("burst limit = %d, want 20", b.Limit)
	}
	if b.RatePerSecond() != 1 {
		t.Errorf("burst rate = %v, want the 1/s the original implied", b.RatePerSecond())
	}
	if err := b.Validate(); err != nil {
		t.Errorf("burst policy is invalid: %v", err)
	}
}

func TestSecondsClamps(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want time.Duration
	}{
		{"negative", -1, 0},
		{"zero", 0, 0},
		{"NaN", math.NaN(), 0},
		{"ordinary", 1.5, 1500 * time.Millisecond},
		// A Duration is nanoseconds in an int64; without the clamp a large
		// enough float wraps and a refusal comes back with a negative wait.
		{"absurd", 1e30, (1 << 33) * time.Second},
		{"infinity", math.Inf(1), (1 << 33) * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := seconds(tc.in); got != tc.want {
				t.Errorf("seconds(%v) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
