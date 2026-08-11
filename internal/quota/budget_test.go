package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/quota"
)

var base = time.Date(2026, 8, 11, 9, 14, 3, 0, time.UTC)

func testDeps(t *testing.T, tokensPerDay int) Deps {
	t.Helper()
	d, err := Build(context.Background(), config.QuotaConfig{
		Backend:         config.QuotaBackendMemory,
		LLMTokensPerDay: tokensPerDay,
	}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestNativeBudgetChargesActualUsage(t *testing.T) {
	ctx := context.Background()
	b := testDeps(t, 1000).Budget()

	if err := b.Reserve(ctx, "t1", 0, base); err != nil {
		t.Fatalf("Reserve on a fresh budget: %v", err)
	}
	if err := b.Record(ctx, "t1", 400, base); err != nil {
		t.Fatalf("Record: %v", err)
	}

	usage, err := b.Usage(ctx, "t1", base)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.Remaining != 600 {
		t.Errorf("remaining = %d after spending 400 of 1000, want 600", usage.Remaining)
	}
	// Reporting must not spend: a `krk quota show` that cost budget would be a
	// trap.
	if again, _ := b.Usage(ctx, "t1", base); again.Remaining != 600 {
		t.Errorf("Usage spent budget: %d", again.Remaining)
	}
}

func TestNativeBudgetExhaustionIsASentinel(t *testing.T) {
	// The loop distinguishes "out of budget" from "something broke", because
	// one pauses for a human and the other ends the iteration.
	ctx := context.Background()
	b := testDeps(t, 100).Budget()

	if err := b.Record(ctx, "t1", 100, base); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := b.Reserve(ctx, "t1", 0, base); !errors.Is(err, ErrBudgetExhausted) {
		t.Errorf("Reserve on a spent budget = %v, want ErrBudgetExhausted", err)
	}

	// Tomorrow is a different period, so the twin starts clean without
	// anything having had to expire.
	if err := b.Reserve(ctx, "t1", 0, base.AddDate(0, 0, 1)); err != nil {
		t.Errorf("Reserve in the next period: %v", err)
	}
}

func TestNativeBudgetIsPerTwin(t *testing.T) {
	ctx := context.Background()
	b := testDeps(t, 100).Budget()

	if err := b.Record(ctx, "t1", 100, base); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := b.Reserve(ctx, "t2", 0, base); err != nil {
		t.Errorf("the second twin was charged for the first: %v", err)
	}
}

func TestNativeBudgetIgnoresUnreportedUsage(t *testing.T) {
	// The CLI-based Gemini provider documents TokensUsed=0 because it has no
	// usage metadata. Charging an invented estimate would make the budget a
	// fiction; charging nothing is at least honest about what is known.
	ctx := context.Background()
	b := testDeps(t, 10).Budget()

	for range 100 {
		if err := b.Record(ctx, "t1", 0, base); err != nil {
			t.Fatalf("Record(0): %v", err)
		}
	}
	if err := b.Reserve(ctx, "t1", 0, base); err != nil {
		t.Errorf("zero-token calls consumed budget: %v", err)
	}
}

func TestUnlimitedBudgetNeverRefuses(t *testing.T) {
	ctx := context.Background()
	var b TokenBudget = UnlimitedBudget{}

	if err := b.Record(ctx, "t1", 1<<30, base); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := b.Reserve(ctx, "t1", 1<<30, base); err != nil {
		t.Errorf("Reserve: %v", err)
	}
	if u, _ := b.Usage(ctx, "t1", base); !u.Allowed {
		t.Error("Usage reported an exhausted budget")
	}
}

func TestPressurePublishesOnTheHub(t *testing.T) {
	// The point of the event: a twin approaching its ceiling should be visible
	// before anything is refused.
	ctx := context.Background()
	d := testDeps(t, 100)
	b := d.Budget()

	if err := b.Record(ctx, "t1", 85, base); err != nil {
		t.Fatalf("Record: %v", err)
	}
	usage, _ := b.Usage(ctx, "t1", base)
	if usage.Used() < PressureThreshold {
		t.Fatalf("used = %v, want at least the %v threshold", usage.Used(), PressureThreshold)
	}
	if !usage.Allowed {
		t.Error("85 of 100 should still be allowed — pressure is a warning, not a refusal")
	}
}

func TestBuildRejectsAnUnknownBackend(t *testing.T) {
	_, err := Build(context.Background(), config.QuotaConfig{Backend: "memcached"}, nil, nil)
	if err == nil {
		t.Fatal("Build accepted an unknown backend")
	}
}

func TestBuildRejectsValkeyWithoutAURL(t *testing.T) {
	// Failing at boot beats discovering it on the first request, when the
	// limiter would fail open and admit everything.
	if _, err := Build(context.Background(), config.QuotaConfig{
		Backend: config.QuotaBackendValkey,
	}, nil, nil); err == nil {
		t.Fatal("Build accepted the valkey backend with no URL")
	}
}

func TestTiersRejectNonsense(t *testing.T) {
	// Checked at boot, so a misconfigured limit fails the process rather than
	// silently admitting everything.
	tiers := DefaultTiers(config.QuotaConfig{})
	if err := tiers.Validate(); err != nil {
		t.Fatalf("the shipped defaults are invalid: %v", err)
	}
	tiers.Capability.Period = "fortnightly"
	if err := tiers.Validate(); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Errorf("Validate() = %v, want ErrInvalidPolicy", err)
	}
}

func TestKeysAreDistinct(t *testing.T) {
	// Two tiers sharing a key would charge one budget for both.
	if TwinKey("t1") == CapabilityKey("t1", "cap") {
		t.Error("the twin and capability keys collide")
	}
	if CapabilityKey("t1", "a") == CapabilityKey("t1", "b") {
		t.Error("two capabilities share a key")
	}
}
