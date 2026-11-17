package loop

// Phase 40 step 5, part B: a re-plan supersedes the checkpoint still open for
// the older plan.
//
// What the code does today, read before writing these:
//   - A loop blocked at a checkpoint in this process cannot plan again: the
//     runner waits on its decision channel. Part A made a restored paused loop
//     wait the same way. The path that does re-plan with a checkpoint open is
//     a crash between stepDecide's cpSvc.Create (decide.go) and the runner's
//     persistState of the paused row (runner.go): the checkpoint is stored
//     pending, the loop row is stored active and names no checkpoint, and
//     ResumeStoredLoops replays the active row from observe. Its replay plans
//     again, escalates, and raises a second checkpoint, leaving the first
//     pending for good with nobody waiting on it.
//   - Nothing closes that first checkpoint. Its status stays pending, it is
//     listed as pending, and an approval of it is recorded as an approval.
//
// Scope: a checkpoint carries its ObjectiveID and TwinID but no loop ID (the
// loop's stored row names the checkpoint, not the other way round), so the
// re-plan supersedes the open checkpoints of the same objective.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// TestSupersededCheckpointClosedByAReplan is the acceptance line: a re-plan
// while cp1 is open closes cp1 as superseded, on the record; only cp2 is
// pending; approving cp1 is refused and runs nothing; cp2 can still be
// approved and runs its own plan.
func TestSupersededCheckpointClosedByAReplan(t *testing.T) {
	ctx := context.Background()

	// The first process plans shownPlan and escalates it as cp1.
	before, _, obj := traceLoopFixture(t)
	before.tracer = nil
	planProcess(t, before, shownPlan)
	run, err := before.Run(ctx, coreloop.Request{
		Objective: obj,
		Agent:     traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: 0}),
		MaxIter:   1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cp1ID := waitAtCheckpoint(t, before, run.LoopID)
	waitStoredPausedAt(t, before.store, run.LoopID, cp1ID)
	cp1, err := before.store.GetCheckpoint(ctx, cp1ID)
	if err != nil {
		t.Fatalf("get cp1: %v", err)
	}
	if cp1.Status != corecheckpoint.StatusPending {
		t.Fatalf("cp1 status = %q before the re-plan, want pending", cp1.Status)
	}

	// The process dies after cp1 was created and before the paused row was
	// stored: the stored loop is active and names no checkpoint.
	st := storedLoop(t, before.store, run.LoopID)
	st.Paused = false
	st.CheckpointID = ""
	if err := before.store.SaveLoopState(ctx, st); err != nil {
		t.Fatalf("store the row a crash leaves: %v", err)
	}

	// The next process replays the active row, and its reasoner plans
	// something else, which escalates as cp2.
	hub := event.NewHub()
	cpSvc := featurecp.NewService(before.store, hub)
	next := &serviceImpl{
		store:  before.store,
		memSvc: before.memSvc,
		cpSvc:  cpSvc,
		hub:    hub,
		otel:   before.otel,
		states: map[string]*loopState{},
	}
	cpSvc.SetResumer(next)
	_, env := planProcess(t, next, replannedPlan)
	if err := next.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	cp2ID := waitAtCheckpoint(t, next, run.LoopID)
	if cp2ID == cp1ID {
		t.Fatalf("the replay waits on cp1 %s; the re-plan raised no checkpoint of its own", cp1ID)
	}
	waitStoredPausedAt(t, next.store, run.LoopID, cp2ID)
	cp2, err := next.store.GetCheckpoint(ctx, cp2ID)
	if err != nil {
		t.Fatalf("get cp2: %v", err)
	}
	if cp2.PlanID == "" || cp2.PlanID == cp1.PlanID {
		t.Fatalf("cp2 names plan %q, want a plan of its own (cp1 names %q)", cp2.PlanID, cp1.PlanID)
	}

	// (a) cp1 is closed as superseded, with the time it was closed.
	got1, err := next.store.GetCheckpoint(ctx, cp1ID)
	if err != nil {
		t.Fatalf("get cp1 after the re-plan: %v", err)
	}
	if got1.Status != corecheckpoint.StatusSuperseded {
		t.Errorf("cp1 status = %q after the re-plan, want %q", got1.Status, corecheckpoint.StatusSuperseded)
	}
	if got1.ResolvedAt == nil {
		t.Error("cp1 carries no resolved time; a superseded checkpoint is closed, and says when")
	}

	// (b) Exactly one audit row records the supersession, and it names both
	// checkpoints. Read by payload as well as kind, so a row of another kind
	// naming both is counted too.
	assertOneSupersessionRow(t, next.store, string(obj.ID), cp1ID, cp2ID)

	// (c) Only cp2 is pending: none is left pending and stale.
	pending := pendingFor(t, next.store, obj)
	if len(pending) != 1 || pending[0].ID != cp2ID {
		t.Errorf("pending checkpoints for the objective = %v, want only cp2 %s", checkpointIDs(pending), cp2ID)
	}

	// (d) Approving cp1 is refused and runs nothing.
	approve := corecheckpoint.Decision{Choice: corecheckpoint.ChoiceApprove, Approver: "user:ada", Note: "run the old plan"}
	if err := next.cpSvc.Resolve(ctx, cp1ID, approve); err == nil {
		t.Error("approving the superseded cp1 returned nil, want an error: a decision on it authorises nothing")
	}
	time.Sleep(stopSettle)
	if ran := env.actions(); len(ran) != 0 {
		t.Errorf("%d action(s) ran after cp1 was approved, want none: %+v", len(ran), ran)
	}
	if after, _ := next.store.GetCheckpoint(ctx, cp1ID); after.Status != corecheckpoint.StatusSuperseded {
		t.Errorf("cp1 status = %q after the refused approval, want it still %q", after.Status, corecheckpoint.StatusSuperseded)
	}
	if got := storedLoop(t, next.store, run.LoopID); !got.Paused || got.CheckpointID != cp2ID {
		t.Errorf("stored loop paused=%v checkpoint=%q after the refused approval, want paused at cp2 %s", got.Paused, got.CheckpointID, cp2ID)
	}

	// (e) cp2 can still be approved, and runs the plan it showed.
	if err := next.cpSvc.Resolve(ctx, cp2ID, approve); err != nil {
		t.Fatalf("approving cp2: %v", err)
	}
	waitStoredCompleted(t, next.store, run.LoopID)
	ran := env.actions()
	if len(ran) != len(cp2.Actions) {
		t.Fatalf("%d action(s) ran after cp2 was approved, want the %d it showed: %+v", len(ran), len(cp2.Actions), ran)
	}
	for i, want := range cp2.Actions {
		if ran[i].capability != want.CapabilityID {
			t.Errorf("action %d ran %q, want %q as cp2 showed", i, ran[i].capability, want.CapabilityID)
		}
	}
	if after, _ := next.store.GetCheckpoint(ctx, cp1ID); after.Status != corecheckpoint.StatusSuperseded {
		t.Errorf("cp1 status = %q after cp2 was approved, want it still %q", after.Status, corecheckpoint.StatusSuperseded)
	}
}

// assertOneSupersessionRow holds the audit log to one row recording that cp1
// was superseded by cp2: of kind supersession, naming both checkpoints.
func assertOneSupersessionRow(t *testing.T, store storage.StorageAdapter, objID, cp1ID, cp2ID string) {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: objID})
	if err != nil {
		t.Fatalf("list audit rows: %v", err)
	}
	var naming, ofKind []storage.ToolEvent
	for _, row := range rows {
		if row.Kind == storage.ToolEventSupersession {
			ofKind = append(ofKind, row)
		}
		if strings.Contains(row.PayloadJSON, cp1ID) && strings.Contains(row.PayloadJSON, cp2ID) {
			naming = append(naming, row)
		}
	}
	if len(ofKind) != 1 {
		t.Errorf("%d audit row(s) of kind %q, want exactly 1", len(ofKind), storage.ToolEventSupersession)
	}
	if len(naming) != 1 {
		t.Errorf("%d audit row(s) name both cp1 %s and cp2 %s, want exactly 1", len(naming), cp1ID, cp2ID)
		return
	}
	row := naming[0]
	if row.Kind != storage.ToolEventSupersession {
		t.Errorf("the row naming both checkpoints is of kind %q, want %q", row.Kind, storage.ToolEventSupersession)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Errorf("supersession payload is not JSON: %v", err)
	}
}
