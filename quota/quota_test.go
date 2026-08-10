package quota

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestQuotaValidate(t *testing.T) {
	tests := []struct {
		name string
		q    Quota
		want string
	}{
		{"daily", Quota{Name: "writes", Cap: 1000, Period: Daily}, ""},
		{"hourly", Quota{Name: "writes", Cap: 10, Period: Hourly}, ""},
		{"monthly", Quota{Name: "spend", Cap: 50, Period: Monthly}, ""},

		{"no name", Quota{Cap: 1, Period: Daily}, "name is empty"},
		{"zero cap", Quota{Name: "writes", Period: Daily}, "cap must be positive"},
		{"unknown period", Quota{Name: "writes", Cap: 1, Period: "weekly"}, "unknown period"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.q.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want an error mentioning %q", err, tc.want)
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Errorf("error does not wrap ErrInvalidPolicy: %v", err)
			}
		})
	}
}

func TestQuotaKeyCarriesThePeriod(t *testing.T) {
	// The period is in the key, which is what makes the reset exact and
	// identical on every backend: at the boundary the key changes, so the new
	// period starts at zero without anything having to notice.
	subject := Key("twin:t1")
	tests := []struct {
		period Period
		want   Key
	}{
		{Hourly, "twin:t1|writes|2026-08-10T14"},
		{Daily, "twin:t1|writes|2026-08-10"},
		{Monthly, "twin:t1|writes|2026-08"},
	}
	for _, tc := range tests {
		t.Run(string(tc.period), func(t *testing.T) {
			q := Quota{Name: "writes", Cap: 10, Period: tc.period}
			if got := q.Key(subject, base); got != tc.want {
				t.Errorf("Key() = %q, want %q", got, tc.want)
			}
		})
	}

	// Local time must not leak in, or two replicas in different zones would
	// disagree about which day it is.
	q := Quota{Name: "writes", Cap: 10, Period: Daily}
	sydney := time.FixedZone("AEST", 10*3600)
	lateUTC := time.Date(2026, 8, 10, 23, 30, 0, 0, time.UTC).In(sydney)
	if got := q.Key(subject, lateUTC); got != "twin:t1|writes|2026-08-10" {
		t.Errorf("Key() in a non-UTC zone = %q, want the UTC day", got)
	}
}

func TestQuotaPeriodEnd(t *testing.T) {
	tests := []struct {
		name   string
		period Period
		now    time.Time
		want   time.Time
	}{
		{"hourly", Hourly, base, time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)},
		{"daily", Daily, base, time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		{"monthly", Monthly, base, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		// Calendar arithmetic, not a nominal 30 days — otherwise a monthly
		// quota drifts a day and a half a year and never rolls on the 1st.
		{"february", Monthly, time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"december rolls the year", Monthly, time.Date(2026, 12, 14, 0, 0, 0, 0, time.UTC),
			time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := Quota{Name: "n", Cap: 1, Period: tc.period}
			if got := q.PeriodEnd(tc.now); !got.Equal(tc.want) {
				t.Errorf("PeriodEnd() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestQuotaTakeAndReset(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryBackend()
	q := Quota{Name: "writes", Cap: 3, Period: Daily}

	for range 3 {
		d, err := q.Take(ctx, b, "twin:t1", 1, base)
		if err != nil {
			t.Fatalf("Take: %v", err)
		}
		if !d.Allowed {
			t.Fatal("refused inside the cap")
		}
	}

	d, err := q.Take(ctx, b, "twin:t1", 1, base)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if d.Allowed {
		t.Fatal("allowed past the cap")
	}
	// The backend's window is deliberately wider than the period, so the
	// reported reset has to come from the calendar rather than from it.
	wantEnd := q.PeriodEnd(base)
	if !d.ResetAt.Equal(wantEnd) {
		t.Errorf("ResetAt = %s, want the period end %s", d.ResetAt, wantEnd)
	}
	if want := wantEnd.Sub(base); d.RetryAfter != want {
		t.Errorf("RetryAfter = %s, want %s", d.RetryAfter, want)
	}

	// Tomorrow is a different key, so the cap is whole again with nothing
	// having had to expire anything.
	if d, _ := q.Take(ctx, b, "twin:t1", 1, base.AddDate(0, 0, 1)); !d.Allowed {
		t.Error("the next period did not start clean")
	}

	// Peek reports without spending.
	if d, _ := q.Peek(ctx, b, "twin:t1", base); d.Allowed {
		t.Error("Peek says there is room in an exhausted period")
	}

	// Reset is the admin override, and it affects this period only.
	if err := q.Reset(ctx, b, "twin:t1", base); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if d, _ := q.Take(ctx, b, "twin:t1", 1, base); !d.Allowed {
		t.Error("Reset did not restore the budget")
	}
}

func TestQuotaRejectsItselfBeforeTouchingTheBackend(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryBackend()
	bad := Quota{Cap: 1, Period: Daily} // no name

	if _, err := bad.Take(ctx, b, "s", 1, base); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("Take error = %v, want ErrInvalidPolicy", err)
	}
	if _, err := bad.Peek(ctx, b, "s", base); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("Peek error = %v, want ErrInvalidPolicy", err)
	}
	if err := bad.Reset(ctx, b, "s", base); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("Reset error = %v, want ErrInvalidPolicy", err)
	}
	if b.Len() != 0 {
		t.Error("an invalid quota reached the backend")
	}
}

func TestQuotaSurfacesBackendErrors(t *testing.T) {
	ctx := context.Background()
	q := Quota{Name: "writes", Cap: 3, Period: Daily}
	boom := errors.New("store unreachable")

	if _, err := q.Take(ctx, failingBackend{boom}, "s", 1, base); !errors.Is(err, boom) {
		t.Errorf("Take error = %v, want the backend's", err)
	}
	if _, err := q.Peek(ctx, failingBackend{boom}, "s", base); !errors.Is(err, boom) {
		t.Errorf("Peek error = %v, want the backend's", err)
	}
}

func TestQuotaPolicyOutlivesThePeriod(t *testing.T) {
	// The key already resets the count, so the policy's window is only a
	// lifetime. Sizing it past the period is what stops a skewed clock
	// expiring a bucket that is still being written to.
	for _, tc := range []struct {
		period  Period
		atLeast time.Duration
	}{
		{Hourly, time.Hour},
		{Daily, 24 * time.Hour},
		{Monthly, 31 * 24 * time.Hour},
	} {
		p := Quota{Name: "n", Cap: 1, Period: tc.period}.policy()
		if p.Algorithm != FixedWindow {
			t.Errorf("%s quota uses %s, want a fixed window", tc.period, p.Algorithm)
		}
		if p.Window < 2*tc.atLeast {
			t.Errorf("%s quota window = %s, want at least two periods", tc.period, p.Window)
		}
	}
}

type failingBackend struct{ err error }

func (f failingBackend) Take(context.Context, Key, Policy, int, time.Time) (Decision, error) {
	return Decision{}, f.err
}

func (f failingBackend) Peek(context.Context, Key, Policy, time.Time) (Decision, error) {
	return Decision{}, f.err
}

func (f failingBackend) Reset(context.Context, Key) error { return f.err }
