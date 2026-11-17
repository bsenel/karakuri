package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 40 step 3, slice B: a pass whose run carried a bounds violation (a
// delegated push outside its declared refs) moves the standing objective to
// blocked, and the supervisor starts no further pass of it until the owner
// resumes it through the existing resume path. The violation arrives as the
// loop row's typed field; the run's error text is worded so that matching it
// could not find the violation.

func violationFixture(t *testing.T) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Status:   objective.StatusActive,
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

func (f *fixture) violate() {
	f.loops.mu.Lock()
	defer f.loops.mu.Unlock()
	f.loops.status = objective.StatusFailed
	f.loops.failure = "the delegated run failed"
	f.loops.violation = &environment.BoundsViolation{
		Reason: "bounds violation: main moved from aaa to bbb, outside the action's declared refs",
		Refs:   []string{"refs/heads/main"},
	}
}

func (f *fixture) objectiveStatus(t *testing.T, id objective.ObjectiveID) objective.ObjectiveStatus {
	t.Helper()
	obj, err := f.store.GetObjective(context.Background(), id)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	return obj.Status
}

func TestAPushOutsideItsRefsBlocksTheStream(t *testing.T) {
	f, obj := violationFixture(t)
	f.violate()

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if got := f.loops.count(); got != 1 {
		t.Fatalf("the loop ran %d times, want 1", got)
	}
	if got := f.objectiveStatus(t, obj.ID); got != objective.StatusBlocked {
		t.Errorf("objective status = %q, want %q after a push outside its refs", got, objective.StatusBlocked)
	}
	st := f.state(t, obj.ID)
	if !st.Paused {
		t.Fatal("the stream is still in rotation after a bounds violation; nothing waits for the owner")
	}
	if st.PausedReason == "" {
		t.Error("the stream was blocked with no reason the owner can read")
	}
}

func TestABlockedStreamRunsNoPassUntilItsOwnerResumesIt(t *testing.T) {
	f, obj := violationFixture(t)
	f.violate()
	f.pass(t, obj.ID, reconcile.TriggerManual)
	if got := f.objectiveStatus(t, obj.ID); got != objective.StatusBlocked {
		t.Fatalf("objective status = %q, want %q", got, objective.StatusBlocked)
	}

	// The next run would be clean; the stream must still wait for the owner.
	f.loops.mu.Lock()
	f.loops.violation, f.loops.failure, f.loops.status = nil, "", objective.StatusCompleted
	f.loops.mu.Unlock()

	// Past the cadence floor, so it is the block and not the floor that
	// holds these passes.
	f.clock = f.clock.Add(2 * time.Hour)
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.pass(t, obj.ID, reconcile.TriggerSchedule)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("a blocked stream ran %d loops, want 1: the supervisor started another pass before the owner looked", got)
	}
	if got := f.objectiveStatus(t, obj.ID); got != objective.StatusBlocked {
		t.Errorf("objective status = %q after passes the owner did not ask for, want %q", got, objective.StatusBlocked)
	}

	if err := f.svc.Resume(context.Background(), obj.ID, "owner"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got := f.objectiveStatus(t, obj.ID); got == objective.StatusBlocked {
		t.Error("the objective is still blocked after its owner resumed it")
	}
	f.clock = f.clock.Add(2 * time.Hour)
	f.pass(t, obj.ID, reconcile.TriggerManual)
	if got := f.loops.count(); got != 2 {
		t.Errorf("after the owner resumed it the stream ran %d loops in all, want 2", got)
	}
}

func TestAPassWithNoViolationLeavesTheStatusAlone(t *testing.T) {
	f, obj := violationFixture(t)
	f.loops.mu.Lock()
	f.loops.status = objective.StatusFailed
	f.loops.failure = "bounds violation: worded like one, but no action carried it"
	f.loops.mu.Unlock()

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if got := f.objectiveStatus(t, obj.ID); got != objective.StatusActive {
		t.Errorf("objective status = %q, want %q: a pass with no violation moved it", got, objective.StatusActive)
	}
	if st := f.state(t, obj.ID); st.Paused {
		t.Errorf("a pass with no violation blocked the stream: %s", st.PausedReason)
	}
}

// The bounds row is the loop's to write, once, when the action's result came
// back. The supervisor blocking the stream adds no second one.
func TestBlockingTheStreamWritesNoSecondBoundsRow(t *testing.T) {
	f, obj := violationFixture(t)
	f.violate()

	f.pass(t, obj.ID, reconcile.TriggerManual)

	violations := true
	rows, err := f.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: string(obj.ID), BoundsViolation: &violations,
	})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("the supervisor wrote %d bounds rows of its own; the loop already wrote the one", len(rows))
	}
}
