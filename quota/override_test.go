package quota_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
)

func TestOverrideValidate(t *testing.T) {
	t.Parallel()

	base := quota.Override{Subject: "principal|alice", Name: "llm-tokens", Cap: 10}
	if err := base.Validate(); err != nil {
		t.Fatalf("a usable override was rejected: %v", err)
	}

	cases := map[string]quota.Override{
		"no subject":      {Name: "llm-tokens", Cap: 10},
		"no name":         {Subject: "principal|alice", Cap: 10},
		"blank name":      {Subject: "principal|alice", Name: "   ", Cap: 10},
		"zero cap":        {Subject: "principal|alice", Name: "llm-tokens"},
		"negative cap":    {Subject: "principal|alice", Name: "llm-tokens", Cap: -1},
		"negative window": {Subject: "principal|alice", Name: "llm-tokens", Cap: 10, Window: -time.Second},
	}
	for name, o := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := o.Validate(); !errors.Is(err, quota.ErrInvalidPolicy) {
				t.Fatalf("err = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestOverrideActive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	// Zero means until somebody removes it, which is what an ordinary raise is.
	if !(quota.Override{}).Active(now) {
		t.Error("an override with no expiry is not active")
	}
	if !(quota.Override{ExpiresAt: now.Add(time.Second)}).Active(now) {
		t.Error("an override expiring in a second is not active")
	}
	if (quota.Override{ExpiresAt: now}).Active(now) {
		t.Error("an override is still active at the instant it expires")
	}
	if (quota.Override{ExpiresAt: now.Add(-time.Second)}).Active(now) {
		t.Error("an expired override is still active")
	}
}

func TestOverrideApply(t *testing.T) {
	t.Parallel()

	base := quota.PerMinute(60).Burst(20) // Limit 20, Rate 1/s
	raised := quota.Override{Cap: 100}.Apply(base)

	if raised.Limit != 100 {
		t.Errorf("Limit = %d, want 100", raised.Limit)
	}
	// The algorithm is not the operator's to change: raising a ceiling is not
	// a decision to swap how traffic is counted.
	if raised.Algorithm != base.Algorithm {
		t.Errorf("Algorithm = %q, want %q", raised.Algorithm, base.Algorithm)
	}
	if raised.Rate != base.Rate {
		t.Errorf("Rate = %v, want the configured %v when only the cap moved", raised.Rate, base.Rate)
	}

	// A new window re-derives the rate. Keeping the old one would refill a
	// bigger bucket at the old speed, which is not the limit anybody approved.
	widened := quota.Override{Cap: 100, Window: time.Hour}.Apply(base)
	if widened.Window != time.Hour {
		t.Errorf("Window = %s, want an hour", widened.Window)
	}
	if widened.Rate != 0 {
		t.Errorf("Rate = %v, want it re-derived from the approved pair", widened.Rate)
	}
	if got := widened.RatePerSecond(); got != 100.0/3600.0 {
		t.Errorf("RatePerSecond = %v, want 100 an hour", got)
	}

	// The base is a value; overriding must not reach back into it.
	if base.Limit != 20 {
		t.Errorf("applying an override mutated the base policy: Limit = %d", base.Limit)
	}
}

func TestOverrideApplyQuota(t *testing.T) {
	t.Parallel()

	base := quota.Quota{Name: "llm-tokens", Cap: 1000, Period: quota.Daily}
	// A window on a quota is ignored: a period is a calendar span, and
	// "every 36 hours" would need a second calendar in the key.
	got := quota.Override{Cap: 5000, Window: 36 * time.Hour}.ApplyQuota(base)

	if got.Cap != 5000 {
		t.Errorf("Cap = %d, want 5000", got.Cap)
	}
	if got.Period != quota.Daily {
		t.Errorf("Period = %q, want it unchanged", got.Period)
	}
	if base.Cap != 1000 {
		t.Errorf("applying an override mutated the base quota: Cap = %d", base.Cap)
	}
}

func TestMemoryOverrideStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := quota.NewMemoryOverrideStore()

	if got, err := s.Overrides(ctx, "principal|alice"); err != nil || got != nil {
		t.Fatalf("Overrides on an empty store = %v, %v", got, err)
	}

	alice := quota.Override{Subject: "principal|alice", Name: "llm-tokens", Cap: 5000}
	if err := s.PutOverride(ctx, alice); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	// Same subject and name replaces rather than accumulating — otherwise a
	// second approval would leave two ceilings and no rule about which wins.
	if err := s.PutOverride(ctx, quota.Override{Subject: "principal|alice", Name: "llm-tokens", Cap: 9000}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	got, err := s.Overrides(ctx, "principal|alice")
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(got) != 1 || got[0].Cap != 9000 {
		t.Fatalf("overrides = %+v, want just the newer cap", got)
	}

	// A returned value must not be a handle on stored state.
	got[0].Cap = 1
	again, _ := s.Overrides(ctx, "principal|alice")
	if again[0].Cap != 9000 {
		t.Fatal("mutating a returned override changed what the store holds")
	}

	if err := s.PutOverride(ctx, quota.Override{Subject: "principal|alice", Name: "capability", Cap: 10}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if err := s.PutOverride(ctx, quota.Override{Subject: "twin|t1", Name: "llm-tokens", Cap: 20}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	all, err := s.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListOverrides = %+v, want three", all)
	}
	// Sorted, so an operator reading the list gets the same order twice.
	if all[0].Subject != "principal|alice" || all[0].Name != "capability" || all[2].Subject != "twin|t1" {
		t.Fatalf("ListOverrides is not in a stable order: %+v", all)
	}

	if err := s.DeleteOverride(ctx, "principal|alice", "llm-tokens"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	if got, _ := s.Overrides(ctx, "principal|alice"); len(got) != 1 || got[0].Name != "capability" {
		t.Fatalf("after delete = %+v, want only the capability override", got)
	}
	// Deleting what is not there is not an error, so a retried revocation is safe.
	if err := s.DeleteOverride(ctx, "principal|alice", "llm-tokens"); err != nil {
		t.Fatalf("second DeleteOverride: %v", err)
	}
	if err := s.DeleteOverride(ctx, "nobody", "nothing"); err != nil {
		t.Fatalf("DeleteOverride on an unknown subject: %v", err)
	}

	if err := s.PutOverride(ctx, quota.Override{Name: "broken"}); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Fatalf("PutOverride of an invalid override = %v, want ErrInvalidPolicy", err)
	}
}
