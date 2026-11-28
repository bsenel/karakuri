package reconcile

// Phase 40 step 14b, the signal read again: after a pass that tried a remedy,
// reconcile reads the blocker's own signal. A signal that no longer shows the
// blocker clears it, and the attempt count with it (a blocker that comes back
// starts at zero). A signal that still shows it makes the pass a failed
// attempt and the count goes up. A remedy whose plan needed a tool outside the
// stream's limits was refused by the bounds: that is a failed attempt too, it
// is never stored as a confirmed remedy, and the limits are left as they were.

import (
	"context"
	"reflect"
	"testing"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

const outsideLimitsRemedy = "run the formatter with the shell tool, which this stream does not have"

func attempted(kind reconcile.BlockerKind, subject string, count, attempts int) reconcile.Blocker {
	b := blk(kind, subject, count, t0)
	b.Attempts = attempts
	return b
}

func blockerOn(t *testing.T, st reconcile.State, kind reconcile.BlockerKind, subject string) reconcile.Blocker {
	t.Helper()
	for _, b := range st.Blockers {
		if b.Kind == kind && b.Subject == subject {
			return b
		}
	}
	t.Fatalf("no blocker %s on %q in %+v", kind, subject, st.Blockers)
	return reconcile.Blocker{}
}

func saveState(t *testing.T, f *fixture, st reconcile.State) {
	t.Helper()
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

// (1) The signal no longer shows the blocker: it is cleared, and the attempts
// spent on it do not follow it into its next occurrence.
func TestAClearedSignalClearsTheBlockerAndResetsItsAttempts(t *testing.T) {
	t.Run("verify is green", func(t *testing.T) {
		f, obj := plainStream(t)
		f.loops.remedy = verifyRemedy
		f.loops.criteria = []objective.Criterion{{ID: "tests-pass", State: objective.CriterionMet}}
		holdBlocker(t, f, obj, attempted(reconcile.BlockerVerifyFailed, "tests-pass", 3, 2))

		f.pass(t, obj.ID, reconcile.TriggerManual)
		wantBlockers(t, f.state(t, obj.ID))

		// It comes back: a new blocker, with no attempt behind it.
		f.failVerify("tests-pass")
		f.loops.remedy = ""
		f.again(t, obj.ID)
		b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass")
		if b.Attempts != 0 {
			t.Errorf("attempts = %d on a blocker that came back after clearing, want 0", b.Attempts)
		}
	})
	t.Run("pull request mergeable", func(t *testing.T) {
		f, obj := plainStream(t)
		f.listUnmergeable("karakuri/phase-99-x")
		f.pass(t, obj.ID, reconcile.TriggerManual)
		st := f.state(t, obj.ID)
		st.Blockers[0].Attempts = 2
		saveState(t, f, st)

		f.loops.remedy = "merge main into the branch, keep both sides, push"
		f.listUnmergeable()
		f.move(branchEnvironment, "tip-after-merge")
		f.again(t, obj.ID)
		wantBlockers(t, f.state(t, obj.ID))

		f.listUnmergeable("karakuri/phase-99-x")
		f.loops.remedy = ""
		f.move(branchEnvironment, "tip-after-main-moved")
		f.again(t, obj.ID)
		if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerUnmergeablePR, "karakuri/phase-99-x"); b.Attempts != 0 {
			t.Errorf("attempts = %d on a branch that became unmergeable again, want 0", b.Attempts)
		}
	})
}

// (2) The signal still shows the blocker: the pass is a failed attempt.
func TestARemedyThatLeavesTheSignalRaisedCountsAsAFailedAttempt(t *testing.T) {
	f, obj := plainStream(t)
	f.failVerify("tests-pass")
	f.loops.remedy = verifyRemedy
	holdBlocker(t, f, obj, attempted(reconcile.BlockerVerifyFailed, "tests-pass", 2, 1))

	f.pass(t, obj.ID, reconcile.TriggerManual)
	if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 2 {
		t.Fatalf("attempts = %d after one more pass that left the signal raised, want 2", b.Attempts)
	}
	f.again(t, obj.ID)
	if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 3 {
		t.Fatalf("attempts = %d after two, want 3", b.Attempts)
	}
	if got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed); len(got) != 0 {
		t.Fatalf("stored %+v for a signal that stayed raised", got)
	}
}

// An attempt is a pass that was given the blocker: another blocker, one the
// pass was not asked to fix, is not charged for it.
func TestOnlyTheBlockerThePassWasGivenIsCountedAsAttempted(t *testing.T) {
	f, obj := plainStream(t)
	f.failVerify("tests-pass", "lint-clean")
	st := f.state(t, obj.ID)
	st.Blockers = []reconcile.Blocker{
		attempted(reconcile.BlockerVerifyFailed, "tests-pass", 5, 0),
		attempted(reconcile.BlockerVerifyFailed, "lint-clean", 1, 0),
	}
	saveState(t, f, st)

	f.pass(t, obj.ID, reconcile.TriggerManual) // given tests-pass: the highest count
	after := f.state(t, obj.ID)
	if b := blockerOn(t, after, reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 1 {
		t.Errorf("tests-pass attempts = %d, want 1", b.Attempts)
	}
	if b := blockerOn(t, after, reconcile.BlockerVerifyFailed, "lint-clean"); b.Attempts != 0 {
		t.Errorf("lint-clean attempts = %d, want 0: the pass was not given it", b.Attempts)
	}
}

// A blocker seen for the first time has had no remedy tried: the pass that
// raised it is not an attempt.
func TestThePassThatRaisesABlockerIsNotAnAttempt(t *testing.T) {
	f, obj := plainStream(t)
	f.failVerify("tests-pass")
	f.pass(t, obj.ID, reconcile.TriggerManual)
	if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 0 {
		t.Fatalf("attempts = %d on a blocker just raised, want 0", b.Attempts)
	}
}

// (3) A fix that needs a tool outside the limits: the bounds refuse the plan.
// The pass is a failed attempt, the blocker stays, nothing is stored as a
// confirmed remedy, and the limits are the ones the pass started with.
func TestARemedyNeedingAToolOutsideTheLimitsIsAFailedAttempt(t *testing.T) {
	f, obj := plainStream(t)
	f.failVerify("tests-pass")
	f.loops.runOutcome = coreloop.RunRefusedByBounds
	f.loops.refused = "action outside the bounds' limits"
	f.loops.remedy = outsideLimitsRemedy
	holdBlocker(t, f, obj, attempted(reconcile.BlockerVerifyFailed, "tests-pass", 2, 0))

	f.pass(t, obj.ID, reconcile.TriggerManual)
	boundsFirst := f.loops.bounds()
	st := f.state(t, obj.ID)
	if b := blockerOn(t, st, reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 1 {
		t.Errorf("attempts = %d after a refused plan, want 1: a failed attempt", b.Attempts)
	}
	if got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed); len(got) != 0 {
		t.Errorf("stored %+v: a refused remedy is not a confirmed one", got)
	}

	f.again(t, obj.ID)
	if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != 2 {
		t.Errorf("attempts = %d after a second refused plan, want 2", b.Attempts)
	}
	if got := f.loops.bounds(); !reflect.DeepEqual(boundsFirst, got) {
		t.Errorf("the limits changed across a refused pass: first %+v, then %+v", boundsFirst, got)
	}
}

// A refusal is not a cleared blocker and its remedy is not confirmed, on a
// blocker whose signal is a listing rather than a criterion.
func TestARefusedRemedyIsNeverStoredForAListedBlocker(t *testing.T) {
	f, obj := plainStream(t)
	f.loops.runOutcome = coreloop.RunRefusedByBounds
	f.loops.refused = "action outside the bounds' limits"
	f.loops.remedy = outsideLimitsRemedy
	f.listUnmergeable("karakuri/phase-99-x")
	holdBlocker(t, f, obj, attempted(reconcile.BlockerUnmergeablePR, "karakuri/phase-99-x", 2, 0))

	f.pass(t, obj.ID, reconcile.TriggerManual)
	st := f.state(t, obj.ID)
	if b := blockerOn(t, st, reconcile.BlockerUnmergeablePR, "karakuri/phase-99-x"); b.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", b.Attempts)
	}
	if got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerUnmergeablePR); len(got) != 0 {
		t.Errorf("stored %+v for a refused remedy", got)
	}
}
