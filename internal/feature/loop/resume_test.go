package loop

import (
	"context"
	"path/filepath"
	"testing"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Both doors onto one act. Until Phase 20 these were disconnected: resolving a
// checkpoint left the loop goroutine blocked forever, and resuming a loop left
// the checkpoint `pending` for good. A standing objective escalates on its own
// schedule and has to carry on afterwards, so it would have wedged on the
// first of those every single time.
func newResumeFixture(t *testing.T) (*serviceImpl, *featurecp.Service, storage.StorageAdapter) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "resume.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	hub := event.NewHub()
	cpSvc := featurecp.NewService(store, hub)
	svc := &serviceImpl{
		store:  store,
		hub:    hub,
		cpSvc:  cpSvc,
		states: map[string]*loopState{},
	}
	cpSvc.SetResumer(svc)
	return svc, cpSvc, store
}

// pausedLoop registers a loop that is sitting at a checkpoint, the way the
// runner leaves it while blocked on <-decisionCh.
func pausedLoop(t *testing.T, svc *serviceImpl, cpSvc *featurecp.Service, loopID string) string {
	t.Helper()
	cp, err := cpSvc.Create(context.Background(), "obj-1", "twin-1",
		"authority_exceeded", "wants to push to main", []string{"approve", "reject"},
		featurecp.CreateOptions{})
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	cpID := cp.ID
	state := &loopState{
		id:         loopID,
		decisionCh: make(chan corecheckpoint.Decision, 1),
		status:     waitingStatus(loopID, true),
		result:     waitingResult(loopID, &cpID),
	}
	svc.mu.Lock()
	svc.states[loopID] = state
	svc.mu.Unlock()
	return cpID
}

func TestResolvingACheckpointUnblocksItsLoop(t *testing.T) {
	svc, cpSvc, store := newResumeFixture(t)
	ctx := context.Background()
	cpID := pausedLoop(t, svc, cpSvc, "loop-a")

	decision := corecheckpoint.Decision{Choice: "approve", Approver: "ada", Note: "ship it"}
	if err := cpSvc.Resolve(ctx, cpID, decision); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// The runner is what normally reads this; here the buffered send standing
	// in the channel is the evidence the decision was delivered.
	svc.mu.RLock()
	state := svc.states["loop-a"]
	svc.mu.RUnlock()
	select {
	case got := <-state.decisionCh:
		if got.Choice != "approve" || got.Approver != "ada" {
			t.Errorf("delivered %+v, want the reviewer's decision", got)
		}
	default:
		t.Fatal("resolving the checkpoint did not deliver a decision to the waiting loop")
	}

	cp, err := store.GetCheckpoint(ctx, cpID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if cp.Status != corecheckpoint.StatusResolved {
		t.Errorf("checkpoint status = %q, want resolved", cp.Status)
	}
}

func TestResumingALoopResolvesItsCheckpoint(t *testing.T) {
	svc, cpSvc, store := newResumeFixture(t)
	ctx := context.Background()
	cpID := pausedLoop(t, svc, cpSvc, "loop-b")

	if _, err := svc.Resume(ctx, "loop-b", corecheckpoint.Decision{Choice: "reject", Approver: "grace"}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	cp, err := store.GetCheckpoint(ctx, cpID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if cp.Status != corecheckpoint.StatusResolved {
		t.Errorf("checkpoint status = %q after resuming its loop, want resolved", cp.Status)
	}
	if cp.Decision == nil || cp.Decision.Choice != "reject" || cp.Decision.Approver != "grace" {
		t.Errorf("checkpoint decision = %+v, want the one passed to Resume", cp.Decision)
	}
}

// Resolve calls Record, and Record must not call back into Resolve. If it did,
// resuming a loop would resolve a checkpoint would resume a loop, forever.
func TestResolveDoesNotRecurse(t *testing.T) {
	svc, cpSvc, _ := newResumeFixture(t)
	ctx := context.Background()
	cpID := pausedLoop(t, svc, cpSvc, "loop-c")

	// A resolve reaches the loop, whose Resume path records the checkpoint
	// again. Completing at all is the assertion.
	if err := cpSvc.Resolve(ctx, cpID, corecheckpoint.Decision{Choice: "approve"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := svc.Resume(ctx, "loop-c", corecheckpoint.Decision{Choice: "approve"}); err == nil {
		t.Error("a second decision was accepted; the buffered channel should refuse it")
	}
}

// A checkpoint nobody is waiting on is an ordinary case, not an error: an
// operator raised it, or its loop is running on another replica.
func TestResolvingACheckpointWithNoWaitingLoop(t *testing.T) {
	_, cpSvc, store := newResumeFixture(t)
	ctx := context.Background()

	cp, err := cpSvc.Create(ctx, "obj-2", "twin-1", "environment_changed", "repo moved",
		[]string{"promote", "dismiss"}, featurecp.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := cpSvc.Resolve(ctx, cp.ID, corecheckpoint.Decision{Choice: "dismiss"}); err != nil {
		t.Fatalf("resolving an unwatched checkpoint failed: %v", err)
	}
	got, err := store.GetCheckpoint(ctx, cp.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != corecheckpoint.StatusResolved {
		t.Errorf("status = %q, want resolved", got.Status)
	}
}

// A decision aimed at a loop that is running rather than waiting is refused,
// not queued for whatever it escalates next.
func TestResumeRefusesALoopThatIsNotWaiting(t *testing.T) {
	svc, _, _ := newResumeFixture(t)
	svc.mu.Lock()
	svc.states["loop-d"] = &loopState{
		id:         "loop-d",
		decisionCh: make(chan corecheckpoint.Decision, 1),
		status:     waitingStatus("loop-d", false),
		result:     waitingResult("loop-d", nil),
	}
	svc.mu.Unlock()

	if _, err := svc.Resume(context.Background(), "loop-d", corecheckpoint.Decision{Choice: "approve"}); err != nil {
		t.Fatalf("the first decision should be accepted into the buffer: %v", err)
	}
	if _, err := svc.Resume(context.Background(), "loop-d", corecheckpoint.Decision{Choice: "approve"}); err == nil {
		t.Error("a second undelivered decision was accepted, want a refusal")
	}
}

func TestResumeCheckpointFindsOnlyThePausedLoop(t *testing.T) {
	svc, cpSvc, _ := newResumeFixture(t)
	ctx := context.Background()
	cpID := pausedLoop(t, svc, cpSvc, "loop-paused")

	// A second loop carrying the same checkpoint ID but not paused. Only the
	// one actually waiting may be handed the decision.
	running := &loopState{
		id:         "loop-running",
		decisionCh: make(chan corecheckpoint.Decision, 1),
		status:     waitingStatus("loop-running", false),
		result:     waitingResult("loop-running", &cpID),
	}
	svc.mu.Lock()
	svc.states["loop-running"] = running
	svc.mu.Unlock()

	found, err := svc.ResumeCheckpoint(ctx, cpID, corecheckpoint.Decision{Choice: "approve"})
	if err != nil || !found {
		t.Fatalf("ResumeCheckpoint: found=%v err=%v", found, err)
	}
	if len(running.decisionCh) != 0 {
		t.Error("the running loop was handed a decision it was not waiting for")
	}

	if found, err := svc.ResumeCheckpoint(ctx, "no-such-checkpoint", corecheckpoint.Decision{Choice: "approve"}); err != nil || found {
		t.Errorf("unknown checkpoint: found=%v err=%v, want false/nil", found, err)
	}
	if found, err := svc.ResumeCheckpoint(ctx, "", corecheckpoint.Decision{Choice: "approve"}); err != nil || found {
		t.Errorf("empty checkpoint id: found=%v err=%v, want false/nil", found, err)
	}
}

// Small constructors so the fixtures above read as intent rather than as
// struct literals.
func waitingStatus(loopID string, paused bool) loop.Status {
	return loop.Status{LoopID: loopID, ObjectiveID: "obj-1", Paused: paused}
}

func waitingResult(loopID string, cpID *string) loop.Result {
	return loop.Result{
		LoopID:       loopID,
		ObjectiveID:  "obj-1",
		Status:       objective.StatusActive,
		CheckpointID: cpID,
	}
}
