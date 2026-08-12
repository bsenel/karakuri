package cost_test

import (
	"testing"

	"github.com/bsenel/karakuri/quota/cost"
	"github.com/bsenel/karakuri/quota/cost/costtest"
)

// The reference implementation, held to the same contract every other ledger
// is — which is what stops a report meaning different things depending on
// which backend is wired up.
func TestSatisfiesContract(t *testing.T) {
	costtest.Run(t, func(t *testing.T) cost.Ledger {
		return cost.NewMemoryLedger()
	})
}

func TestMemoryLedgerLen(t *testing.T) {
	l := cost.NewMemoryLedger()
	if l.Len() != 0 {
		t.Fatalf("Len = %d on a new ledger", l.Len())
	}
	if err := l.Record(t.Context(), cost.Event{
		Subject: "twin|t1", Units: 1, UnitKind: cost.UnitTokens, OccurredAt: costtest.Base,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if l.Len() != 1 {
		t.Fatalf("Len = %d after one event", l.Len())
	}
}

// A caller must not be able to mutate stored state through the slice it passed.
func TestMemoryLedgerClonesLabels(t *testing.T) {
	l := cost.NewMemoryLedger()
	labels := []string{"org:o_acme"}
	if err := l.Record(t.Context(), cost.Event{
		Subject: "twin|t1", Units: 1, UnitKind: cost.UnitTokens,
		OccurredAt: costtest.Base, Labels: labels,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	labels[0] = "org:o_globex"

	got, err := l.Aggregate(t.Context(), cost.Query{Labels: []string{"org:o_acme"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 {
		t.Fatal("mutating the caller's slice changed what the ledger holds")
	}
}
