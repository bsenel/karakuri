package report

// Phase 40 step 14c: a stream that spent its attempts on a blocker shows, in
// the oversight view, "blocked on <diagnosis>, tried <what>" with the number of
// attempts, so it ends with a diagnosis in the view and not with a breaker
// checkpoint that has none. A blocker still being tried shows no such line.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

const handedDiagnosis = "TestClock reads time.Now and the fixture pins no clock"

func blockOn(blocks []digest.Block, kind reconcile.BlockerKind, subject string) (digest.Block, bool) {
	for _, b := range blocks {
		if b.Kind == digest.BlockKind(kind) && b.Subject == subject {
			return b, true
		}
	}
	return digest.Block{}, false
}

func TestTheViewShowsASpentBlockerAsBlockedOnItsDiagnosisWithWhatWasTried(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	a := seedStanding(t, store, "twin-1", "obj-a", "stream a")

	until := *clock
	since := until.Add(-24 * time.Hour)
	seen := since.Add(2 * time.Hour)
	tried := []reconcile.Attempt{
		{LoopID: "loop-1", At: seen.Add(1 * time.Hour), Diagnosis: handedDiagnosis, Remedy: "pin the clock in the fixture", AuditRows: []string{"row-1"}},
		{LoopID: "loop-2", At: seen.Add(2 * time.Hour), Diagnosis: handedDiagnosis, Remedy: "inject a clock into the service", AuditRows: []string{"row-2"}},
		{LoopID: "loop-3", At: seen.Add(3 * time.Hour), Diagnosis: handedDiagnosis, Remedy: "freeze time in TestMain", AuditRows: []string{"row-3"}},
	}
	err := store.SaveReconcileState(ctx, reconcile.State{
		ObjectiveID: a.ID, TwinID: "twin-1", Phase: reconcile.PhaseIdle,
		Blockers: []reconcile.Blocker{
			{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-pass", Count: 4, Attempts: 3, Tried: tried, FirstSeen: seen, LastSeen: seen.Add(3 * time.Hour)},
			{Kind: reconcile.BlockerUnmergeablePR, Subject: "karakuri/phase-40-x", Count: 2, Attempts: 1, FirstSeen: seen, LastSeen: seen},
		},
		Handovers: []reconcile.Handover{{
			Kind: reconcile.BlockerVerifyFailed, Subject: "tests-pass", Reason: reconcile.HandoverAttemptsSpent,
			Diagnosis: handedDiagnosis, Attempts: tried, At: seen.Add(3 * time.Hour),
		}},
	})
	if err != nil {
		t.Fatalf("seed state: %v", err)
	}

	d, err := svc.Assemble(ctx, "twin-1", since, until)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	sum := summaryOf(t, d, "obj-a")

	spent, ok := blockOn(sum.Blocks, reconcile.BlockerVerifyFailed, "tests-pass")
	if !ok {
		t.Fatalf("blocks = %+v, want the spent verify_failed blocker", sum.Blocks)
	}
	if want := "blocked on " + handedDiagnosis + ", tried "; !strings.HasPrefix(spent.Reason, want) {
		t.Errorf("reason = %q, want it to start %q", spent.Reason, want)
	}
	for _, a := range tried {
		if !strings.Contains(spent.Reason, a.Remedy) {
			t.Errorf("reason = %q does not name what was tried: %q", spent.Reason, a.Remedy)
		}
	}
	if spent.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", spent.Attempts)
	}

	trying, ok := blockOn(sum.Blocks, reconcile.BlockerUnmergeablePR, "karakuri/phase-40-x")
	if !ok {
		t.Fatalf("blocks = %+v, want the unmergeable_pr blocker still being tried", sum.Blocks)
	}
	if strings.HasPrefix(trying.Reason, "blocked on") {
		t.Errorf("a blocker still being tried reads %q: it is not handed over", trying.Reason)
	}
	if trying.Attempts != 1 {
		t.Errorf("attempts = %d on the blocker being tried, want 1", trying.Attempts)
	}
}
