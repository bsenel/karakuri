package storage_test

import (
	"context"
	"testing"
	"time"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 41 step 8 part B: sustain is evaluated over recorded passes, so the
// per-criterion states an outcome carries, and the sustain state a reconcile
// state carries beside ScoreStreak, must survive the database.

func TestReconcileOutcomeCriteriaRoundTrip(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

	want := []coreobjective.CriterionPass{
		{CriterionID: "uptime", State: coreobjective.CriterionMet, At: at},
		{CriterionID: "tested", State: coreobjective.CriterionInconclusive, Reason: coreobjective.ReasonVerifierError, At: at},
	}
	if err := s.SaveReconcileOutcome(ctx, reconcile.Outcome{
		ID: "o-1", ObjectiveID: "obj", Trigger: reconcile.TriggerManual, LoopID: "loop-1",
		Inconclusive: true, Criteria: want, StartedAt: at, EndedAt: at,
	}); err != nil {
		t.Fatalf("save outcome: %v", err)
	}
	// A sense-only pass settled nothing, and must not read back as if it had.
	if err := s.SaveReconcileOutcome(ctx, reconcile.Outcome{
		ID: "o-0", ObjectiveID: "obj", StartedAt: at.Add(-time.Hour), EndedAt: at.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save sense outcome: %v", err)
	}

	got, err := s.ListReconcileOutcomes(ctx, "obj", 10)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(got) != 2 || got[0].ID != "o-1" {
		t.Fatalf("outcomes = %d, want o-1 then o-0", len(got))
	}
	if len(got[0].Criteria) != len(want) {
		t.Fatalf("criteria = %+v, want %+v", got[0].Criteria, want)
	}
	for i := range want {
		g := got[0].Criteria[i]
		if g.CriterionID != want[i].CriterionID || g.State != want[i].State ||
			g.Reason != want[i].Reason || !g.At.Equal(want[i].At) {
			t.Errorf("criteria[%d] = %+v, want %+v", i, g, want[i])
		}
	}
	if len(got[1].Criteria) != 0 {
		t.Errorf("a pass saved with no criteria read back with %+v", got[1].Criteria)
	}
}

func TestReconcileStateSustainRoundTrip(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	want := []reconcile.CriterionSustain{
		{
			CriterionID: "uptime", State: coreobjective.CriterionMet,
			Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: coreobjective.CriterionInconclusive,
			WindowStart: start,
		},
		{
			CriterionID: "latency", State: coreobjective.CriterionInconclusive, Reason: coreobjective.ReasonTooFewSamples,
			Settled: 2, NotMet: 0, BudgetLeft: 0.25, Latest: coreobjective.CriterionMet,
			WindowStart: start.Add(14 * 24 * time.Hour),
		},
	}
	if err := s.SaveReconcileState(ctx, reconcile.State{
		ObjectiveID: "obj", Phase: reconcile.PhaseIdle, ScoreStreak: 2, Sustain: want,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetReconcileState(ctx, "obj")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ScoreStreak != 2 {
		t.Errorf("score_streak = %d, want 2 beside the sustain state", got.ScoreStreak)
	}
	if len(got.Sustain) != len(want) {
		t.Fatalf("sustain = %+v, want %+v", got.Sustain, want)
	}
	for i := range want {
		g, w := got.Sustain[i], want[i]
		if g.CriterionID != w.CriterionID || g.State != w.State || g.Reason != w.Reason ||
			g.Settled != w.Settled || g.NotMet != w.NotMet || g.BudgetLeft != w.BudgetLeft ||
			g.Latest != w.Latest || !g.WindowStart.Equal(w.WindowStart) {
			t.Errorf("sustain[%d] = %+v, want %+v", i, g, w)
		}
	}

	// The due-wheel query maps rows the same way Get does.
	if err := s.SaveReconcileState(ctx, reconcile.State{
		ObjectiveID: "due", NextDueAt: ptr(start), Sustain: want[:1],
	}); err != nil {
		t.Fatalf("save due: %v", err)
	}
	due, err := s.ListDueReconcileStates(ctx, "server-a", start.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || len(due[0].Sustain) != 1 || due[0].Sustain[0].CriterionID != "uptime" {
		t.Errorf("due states = %+v, want obj 'due' with its uptime sustain", due)
	}
}
