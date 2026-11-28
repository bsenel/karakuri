package storage_test

import (
	"context"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 40 step 14b: the remedy a pass applied survives the loop row's round
// trip, because reconcile reads it from there.
func TestLoopStateRemedyRoundTrips(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	if err := s.SaveLoopState(ctx, coreloop.State{LoopID: "l2", ObjectiveID: "obj", Remedy: "pin the clock"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetLoopState(ctx, "l2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Remedy != "pin the clock" {
		t.Errorf("remedy = %q, want it kept", got.Remedy)
	}
}

// Phase 40 step 14b: semantic recall filters by domain, which is where a
// remedy keeps its blocker's kind. A store that ignores it hands every kind's
// remedies to every blocker.
func TestQuerySemanticFiltersByDomain(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	for id, domain := range map[string]string{"a": "verify_failed", "b": "plan_error"} {
		if err := s.SaveMemorySemantic(ctx, memory.Entry{ID: id, AgentID: "ag", Domain: domain, Content: "remedy " + id}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	got, err := s.QuerySemantic(ctx, memory.Query{AgentID: "ag", Domain: "verify_failed"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("entries = %+v, want only the verify_failed one", got)
	}
}

// Phase 40 step 14a: the reason the bounds refused a plan survives the loop
// row's round trip, because reconcile reads it from there.
func TestLoopStatePlanRefusedReasonRoundTrips(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	if err := s.SaveLoopState(ctx, coreloop.State{LoopID: "l1", ObjectiveID: "obj", PlanRefusedReason: "outside the limits"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetLoopState(ctx, "l1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PlanRefusedReason != "outside the limits" {
		t.Errorf("plan_refused_reason = %q, want it kept", got.PlanRefusedReason)
	}
}

// Phase 40 step 14a: a stream's blockers and the no_progress counters survive
// the database.
func TestReconcileStateBlockersRoundTrip(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	want := []reconcile.Blocker{
		{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-green", Count: 2, Attempts: 2, FirstSeen: at, LastSeen: at.Add(time.Hour)},
		{Kind: reconcile.BlockerDeferredQuota, Subject: "obj", Count: 1, FirstSeen: at, LastSeen: at},
	}
	if err := s.SaveReconcileState(ctx, reconcile.State{
		ObjectiveID: "obj", Phase: reconcile.PhaseIdle, Blockers: want, BranchSHA: "abc", StalledPasses: 2,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetReconcileState(ctx, "obj")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Blockers) != len(want) {
		t.Fatalf("blockers = %+v, want %+v", got.Blockers, want)
	}
	for i, w := range want {
		g := got.Blockers[i]
		if g.Kind != w.Kind || g.Subject != w.Subject || g.Count != w.Count || g.Attempts != w.Attempts || !g.FirstSeen.Equal(w.FirstSeen) || !g.LastSeen.Equal(w.LastSeen) {
			t.Errorf("blockers[%d] = %+v, want %+v", i, g, w)
		}
	}
	if got.BranchSHA != "abc" || got.StalledPasses != 2 {
		t.Errorf("branch_sha %q stalled_passes %d, want abc and 2", got.BranchSHA, got.StalledPasses)
	}
}
