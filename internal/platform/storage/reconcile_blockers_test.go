package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 40 step 14a: a stream's blockers and the no_progress counters survive
// the database.
func TestReconcileStateBlockersRoundTrip(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	want := []reconcile.Blocker{
		{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-green", Count: 2, FirstSeen: at, LastSeen: at.Add(time.Hour)},
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
		if g.Kind != w.Kind || g.Subject != w.Subject || g.Count != w.Count || !g.FirstSeen.Equal(w.FirstSeen) || !g.LastSeen.Equal(w.LastSeen) {
			t.Errorf("blockers[%d] = %+v, want %+v", i, g, w)
		}
	}
	if got.BranchSHA != "abc" || got.StalledPasses != 2 {
		t.Errorf("branch_sha %q stalled_passes %d, want abc and 2", got.BranchSHA, got.StalledPasses)
	}
}
