package loop

import (
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// flatPricer charges one unit of currency per token, so a test can reason
// about the ceiling without reproducing a price table.
type flatPricer struct{}

func (flatPricer) Price(_, _, _ string, units float64) float64 { return units }

func meteredAgent(t *testing.T) *budgetedAgent {
	t.Helper()
	return &budgetedAgent{
		twinID: "twin-1",
		costs:  &karakuriquota.Recorder{Pricer: flatPricer{}},
	}
}

// Budget.PerReconcile was declared in Phase 23 and read by nothing: the daily
// ceiling was enforced and this one held a plausible number that changed no
// behaviour. It bounds the blast radius of one pass that goes wrong — a run
// that spends a day's allowance in a single pass has stayed inside its daily
// ceiling and still wants stopping.
func TestThePerPassCeilingBites(t *testing.T) {
	obj := objective.Objective{
		ID:     "obj-1",
		Budget: &objective.Budget{PerReconcile: 100},
	}
	a := meteredAgent(t)

	if _, _, over := perPassCeilingReached(a, obj); over {
		t.Fatal("a run that has spent nothing was already over its ceiling")
	}

	a.spent = 99
	if _, _, over := perPassCeilingReached(a, obj); over {
		t.Error("a run under its ceiling was stopped")
	}

	// Reached, not passed — matching Budget.ExceedsDaily. A ledger is written
	// after the work, so a run sitting exactly on its ceiling has already
	// spent it; treating that as room left rounds the ceiling up by one
	// iteration every pass.
	a.spent = 100
	spent, ceiling, over := perPassCeilingReached(a, obj)
	if !over {
		t.Error("a run sitting exactly on its ceiling was given another iteration")
	}
	if spent != 100 || ceiling != 100 {
		t.Errorf("reported spent=%v ceiling=%v, want 100/100", spent, ceiling)
	}
}

// An objective that declared no per-pass ceiling is unaffected, which is every
// objective written before Phase 23.
func TestNoPerPassCeilingMeansNoCeiling(t *testing.T) {
	a := meteredAgent(t)
	a.spent = 1e9

	for _, obj := range []objective.Objective{
		{ID: "no-budget-at-all"},
		{ID: "daily-only", Budget: &objective.Budget{Daily: 5}},
		{ID: "explicit-zero", Budget: &objective.Budget{PerReconcile: 0}},
	} {
		if _, _, over := perPassCeilingReached(a, obj); over {
			t.Errorf("objective %q was stopped by a ceiling it never declared", obj.ID)
		}
	}
}

// An unmetered loop — no twin, so nothing wrapped the agent — cannot be
// charged and must not be stopped. Reporting "over budget" for a run whose
// spend is unknown would stop objectives for a reason unrelated to their
// budget.
func TestAnUnmeteredRunHasNoCeiling(t *testing.T) {
	obj := objective.Objective{ID: "obj-1", Budget: &objective.Budget{PerReconcile: 1}}
	if _, _, over := perPassCeilingReached(nil, obj); over {
		t.Error("a run with no meter was stopped by a ceiling it cannot measure")
	}
}

// The running total and the eventual bill have to agree, or the ceiling stops
// a run at a number nobody will find in the ledger.
func TestTheRunningTotalIsPricedLikeTheLedger(t *testing.T) {
	a := meteredAgent(t)
	spend := karakuriquota.Spend{
		TwinID: "twin-1", Provider: "test", Model: "test",
		Units: 42, UnitKind: cost.UnitTokens,
	}
	if got := a.costs.Price(spend); got != 42 {
		t.Errorf("Price = %v, want 42", got)
	}

	// And with no pricer, a spend prices at zero — the same answer Record
	// would write — rather than panicking or guessing.
	unpriced := &karakuriquota.Recorder{}
	if got := unpriced.Price(spend); got != 0 {
		t.Errorf("Price with no pricer = %v, want 0", got)
	}
}
