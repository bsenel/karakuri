package quota_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/event"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

type stubScopes struct {
	labels map[string][]string
	err    error
}

func (s *stubScopes) ScopesOf(_ context.Context, resourceType, resourceID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.labels[resourceType+":"+resourceID], nil
}

func recorder(t *testing.T, containers karakuriquota.ScopeLookup) (*karakuriquota.Recorder, *cost.MemoryLedger) {
	t.Helper()
	ledger := cost.NewMemoryLedger()
	return &karakuriquota.Recorder{
		Ledger: ledger,
		Pricer: cost.NewStaticPricer([]cost.Rate{
			{Provider: "anthropic", Model: "opus", UnitKind: cost.UnitTokens, PerUnit: 0.000015},
		}),
		Containers: containers,
		Now:        func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) },
	}, ledger
}

func TestRecorderPricesAndLabels(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{labels: map[string][]string{
		"twin:t1": {"team:t_eng", "org:o_acme"},
	}})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{GroupBy: []cost.GroupBy{cost.ByLabel}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("buckets = %+v, want the team and the org", got)
	}
	// Priced from the configured table.
	for _, b := range got {
		if b.Cost != 15 {
			t.Fatalf("bucket %v cost %v, want 15", b.Key, b.Cost)
		}
	}
}

// An objective inherits its twin's containers when it has none of its own,
// which is what puts a tool call's spend in the right team on a deployment
// where objectives were never placed explicitly.
func TestRecorderFallsBackToTheTwinsContainers(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{labels: map[string][]string{
		"twin:t1": {"org:o_acme"},
	}})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", ResourceType: "objective", ResourceID: "obj-1",
		Provider: "github", Units: 1, UnitKind: cost.UnitCalls,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{Labels: []string{"org:o_acme"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 || got[0].Events != 1 {
		t.Fatalf("buckets = %+v, want the objective under its twin's org", got)
	}
}

// The spend happened whatever the tree says, so an unreadable container service
// costs the per-team dimension and not the money.
func TestRecorderRecordsWhenTheTreeCannotBeRead(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{err: errors.New("database down")})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 || got[0].Cost != 15 {
		t.Fatalf("buckets = %+v, want the spend recorded unlabelled", got)
	}
}

// Providers that cannot report usage send zero. Recording those would fill the
// ledger with rows that say nothing.
func TestRecorderIgnoresZeroUnits(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, nil)

	r.Record(ctx, karakuriquota.Spend{TwinID: "t1", Provider: "gemini-cli", Units: 0, UnitKind: cost.UnitTokens})
	if ledger.Len() != 0 {
		t.Fatalf("a zero-unit spend was recorded")
	}
}

// A deployment with no ledger must not make the loop branch.
func TestRecorderZeroValueDiscards(t *testing.T) {
	var r *karakuriquota.Recorder
	r.Record(context.Background(), karakuriquota.Spend{TwinID: "t1", Units: 100, UnitKind: cost.UnitTokens})
	if r.Enabled() {
		t.Error("a nil recorder reported itself enabled")
	}
	empty := &karakuriquota.Recorder{}
	empty.Record(context.Background(), karakuriquota.Spend{TwinID: "t1", Units: 100, UnitKind: cost.UnitTokens})
	if empty.Enabled() {
		t.Error("a recorder with no ledger reported itself enabled")
	}
}

// A dashboard follows the hub, so every write has to publish.
func TestRecorderPublishes(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub()
	r, _ := recorder(t, nil)
	r.Hub = hub

	sub, unsub := hub.Subscribe(ctx, "_global")
	defer unsub()
	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	select {
	case ev := <-sub:
		if ev.Type != event.TypeCostRecorded {
			t.Fatalf("event type = %q", ev.Type)
		}
		if ev.Payload["cost"] != 15.0 {
			t.Fatalf("payload cost = %v, want 15", ev.Payload["cost"])
		}
	case <-time.After(time.Second):
		t.Fatal("no cost_recorded event was published")
	}
}
