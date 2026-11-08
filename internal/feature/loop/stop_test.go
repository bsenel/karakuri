package loop

// Phase 37 step 3, slice A: stopping a one-shot loop in the loop service.
//
// What the code does today, read before writing these (line numbers as read
// on 2026-11-08):
//   - Run (service.go line 210) starts runLoop on context.Background() and
//     keeps no cancel function, so nothing outside the loop can cancel it.
//   - runLoop (runner.go) looks at ctx.Done() only while it waits at a
//     checkpoint; a loop that is acting or reasoning carries on through its
//     iterations. Each iteration boundary persists the state with
//     Completed=false (runner.go line 380), and finalizeLoop persists it with
//     Completed=true (line 530), which is what keeps a row out of
//     ListActiveLoopStates (gorm_storage.go line 779, `completed = false`).
//   - A loop that ends on a cancelled context goes through finalizeLoop with
//     ctx.Err(): its row reads status failed, outcome errored, and Error
//     carries the context's error text, which is how reconcile reads a failed
//     run. A stop must not read that way.
//   - checkpoint.Service.Resolve (feature/checkpoint/service.go line 138)
//     records a decision on any checkpoint, pending or not, and Resume
//     delivers into a buffered channel whether or not a runner is reading it.
//   - Watch mode is not a loop the loop service holds: POST /loops with
//     watch_mode declares the objective standing at sense autonomy
//     (internal/api/handler/loop.go, declareWatch) and then calls Run, whose
//     one-shot loop ends. The wait between events is reconcile's tick, not a
//     goroutine here. The watch-mode test below is kept as the step asks for
//     it and fails with that message until the stop reaches it.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const (
	stopPrincipal = "user:ada"
	stopReason    = "started against a branch that was just merged"

	// stopBound is the phase's working bound from the acknowledgement to the
	// action in flight ending (docs/roadmap.md, Phase 37).
	stopBound = 10 * time.Second

	// stopSettle is long enough for a runner that was wrongly left going to
	// start its next action or overwrite the stored row.
	stopSettle = 300 * time.Millisecond
)

// The outcome a stopped run ends with: a reason of its own, as step 1 named
// the stopped pass (docs/roadmap.md, Phase 37 step 1).
const wantStoppedOutcome = coreloop.RunOutcome("stopped")

// stopEnv counts the actions that start and, when block is set, holds each
// one until its context is cancelled, the way a delegated coding action holds
// the loop while its process runs.
type stopEnv struct {
	traceEnv
	block bool

	mu        sync.Mutex
	starts    int
	started   chan struct{}
	cancelled chan struct{}
}

func newStopEnv(block bool) *stopEnv {
	return &stopEnv{
		traceEnv:  traceEnv{id: "test.env.ok"},
		block:     block,
		started:   make(chan struct{}, 16),
		cancelled: make(chan struct{}, 16),
	}
}

func (e *stopEnv) Act(ctx context.Context, _ environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	e.starts++
	e.mu.Unlock()
	e.started <- struct{}{}
	if !e.block {
		return environment.ActionResult{Success: true}, nil
	}
	<-ctx.Done()
	e.cancelled <- struct{}{}
	return environment.ActionResult{}, ctx.Err()
}

func (e *stopEnv) startCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.starts
}

func (e *stopEnv) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-e.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop's first action never started")
	}
}

func (e *stopEnv) waitCancelled(t *testing.T) {
	t.Helper()
	select {
	case <-e.cancelled:
	case <-time.After(stopBound):
		t.Fatalf("the action in flight did not see its context cancelled within %s of Stop returning", stopBound)
	}
}

func stopEnvRegistry(t *testing.T, env *stopEnv) *environment.Registry {
	t.Helper()
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  "test.env.ok",
		Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return env, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return reg
}

// stopFixture is a service that runs the whole loop over the scripted
// provider, every action of which goes to env. The objective has no success
// criteria, so without a stop the loop runs all five iterations.
func stopFixture(t *testing.T, env *stopEnv, bounds coreagent.AuthorityBounds) (*serviceImpl, coreloop.Request) {
	t.Helper()
	svc, _, obj := traceLoopFixture(t)
	svc.tracer = nil
	svc.envReg = stopEnvRegistry(t, env)
	return svc, coreloop.Request{Objective: obj, Agent: traceAgent(bounds), MaxIter: 5}
}

// restarted is a new process over the same store: nothing in memory, and
// its own environment so any action it starts is counted apart.
func restarted(t *testing.T, svc *serviceImpl, env *stopEnv) *serviceImpl {
	t.Helper()
	return &serviceImpl{
		store:   svc.store,
		factory: svc.factory,
		envReg:  stopEnvRegistry(t, env),
		memSvc:  svc.memSvc,
		cpSvc:   svc.cpSvc,
		hub:     event.NewHub(),
		otel:    svc.otel,
		states:  map[string]*loopState{},
	}
}

var unlimited = coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions}

func storedLoop(t *testing.T, store storage.StorageAdapter, loopID string) coreloop.State {
	t.Helper()
	st, err := store.GetLoopState(context.Background(), loopID)
	if err != nil {
		t.Fatalf("get loop state %s: %v", loopID, err)
	}
	return st
}

func waitStoredCompleted(t *testing.T, store storage.StorageAdapter, loopID string) coreloop.State {
	t.Helper()
	deadline := time.Now().Add(stopBound)
	for {
		st, err := store.GetLoopState(context.Background(), loopID)
		if err == nil && st.Completed {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("loop %s did not end within %s: completed=%v outcome=%q (err %v)", loopID, stopBound, st.Completed, st.Outcome, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// assertStoppedRow is what a stopped loop's stored row must say: ended, so
// no restart lists it; stopped, as its own outcome; and who stopped it and
// why. Where the principal and the reason live in the row is the
// implementation's choice, so the row is read whole.
func assertStoppedRow(t *testing.T, st coreloop.State, when string) {
	t.Helper()
	if !st.Completed {
		t.Errorf("%s: stored loop completed = false; a restart's ListActiveLoopStates would list it and replay it", when)
	}
	if st.Outcome != wantStoppedOutcome {
		t.Errorf("%s: stored outcome = %q, want %q", when, st.Outcome, wantStoppedOutcome)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal stored state: %v", err)
	}
	if !strings.Contains(string(raw), stopPrincipal) {
		t.Errorf("%s: the stored loop does not carry the principal %q who stopped it", when, stopPrincipal)
	}
	if !strings.Contains(string(raw), stopReason) {
		t.Errorf("%s: the stored loop does not carry the reason %q it was stopped for", when, stopReason)
	}
}

// waitAtCheckpoint returns the checkpoint a running loop is waiting on.
func waitAtCheckpoint(t *testing.T, svc *serviceImpl, loopID string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		svc.mu.RLock()
		state := svc.states[loopID]
		svc.mu.RUnlock()
		if state != nil {
			state.mu.RLock()
			paused, cpID := state.status.Paused, state.result.CheckpointID
			state.mu.RUnlock()
			if paused && cpID != nil && *cpID != "" {
				return *cpID
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the loop never stopped at a checkpoint")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// (1) The acceptance line: a loop whose action is in flight is stopped. The
// action's context is cancelled, the loop ends as stopped with who and why,
// and no further action starts once Stop has answered.
func TestStopCancelsARunningLoopAndFinalisesItAsStopped(t *testing.T) {
	env := newStopEnv(true)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	env.waitStarted(t)

	got, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	startsAtStop := env.startCount()
	if got.LoopID != run.LoopID || got.Found != StopFoundRunning {
		t.Errorf("Stop = %+v, want loop %s found %q", got, run.LoopID, StopFoundRunning)
	}

	// Stored as stopped before the cancel, so the row already says so when
	// Stop answers.
	assertStoppedRow(t, storedLoop(t, svc.store, run.LoopID), "when Stop returned")

	env.waitCancelled(t)
	time.Sleep(stopSettle)

	if n := env.startCount(); n != startsAtStop {
		t.Errorf("%d action(s) started after Stop returned, want none", n-startsAtStop)
	}
	// The runner unwinding from the cancelled action must not write over
	// the stopped row: not an iteration boundary with completed=false, and
	// not a finalize that reads the cancel as a failure.
	assertStoppedRow(t, storedLoop(t, svc.store, run.LoopID), "after the runner unwound")
}

// (2) A stopped loop stays stopped: a new process over the same store
// replays nothing.
func TestAStoppedLoopIsNotReplayedByARestart(t *testing.T) {
	env := newStopEnv(true)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	env.waitStarted(t)
	if _, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	env.waitCancelled(t)
	time.Sleep(stopSettle)

	after := newStopEnv(false)
	next := restarted(t, svc, after)
	if err := next.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	time.Sleep(stopSettle)

	next.mu.RLock()
	_, registered := next.states[run.LoopID]
	next.mu.RUnlock()
	if registered {
		t.Error("the restart registered the stopped loop; it must not be brought back")
	}
	if n := after.startCount(); n != 0 {
		t.Errorf("the restart started %d action(s) for a stopped loop, want none", n)
	}
	active, err := svc.store.ListActiveLoopStates(ctx)
	if err != nil {
		t.Fatalf("ListActiveLoopStates: %v", err)
	}
	for _, st := range active {
		if st.LoopID == run.LoopID {
			t.Errorf("ListActiveLoopStates still lists the stopped loop %s", run.LoopID)
		}
	}
}

// (3) A loop waiting at a checkpoint is stopped: it ends, and a later
// approval of that checkpoint is refused and authorises nothing.
func TestStoppingALoopAtACheckpointMakesALaterApprovalAuthoriseNothing(t *testing.T) {
	env := newStopEnv(false)
	// No action may be taken on the agent's own authority, so the first plan
	// escalates.
	svc, req := stopFixture(t, env, coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cpID := waitAtCheckpoint(t, svc, run.LoopID)

	got, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got.Found != StopFoundRunning {
		t.Errorf("Stop found a loop waiting at a checkpoint %q, want %q: it had not ended", got.Found, StopFoundRunning)
	}
	assertStoppedRow(t, waitStoredCompleted(t, svc.store, run.LoopID), "after stopping at a checkpoint")

	approve := corecheckpoint.Decision{Choice: "approve", Approver: "user:grace", Note: "go ahead"}
	if err := svc.cpSvc.Resolve(ctx, cpID, approve); err == nil {
		t.Error("approving the checkpoint of a stopped loop was accepted; it must be refused")
	}
	if _, err := svc.Resume(ctx, run.LoopID, approve); err == nil {
		t.Error("resuming a stopped loop with an approval was accepted; it must be refused")
	}
	time.Sleep(stopSettle)

	if n := env.startCount(); n != 0 {
		t.Errorf("%d action(s) started after the stopped loop's checkpoint was approved, want none", n)
	}
	if cp, err := svc.store.GetCheckpoint(ctx, cpID); err != nil {
		t.Errorf("get checkpoint: %v", err)
	} else if cp.Decision != nil && cp.Decision.Choice == "approve" {
		t.Errorf("the stopped loop's checkpoint records an approval by %q; a stop leaves nothing to approve", cp.Decision.Approver)
	}
	if evs := objectiveEvents(t, svc.store, req.Objective, storage.ToolEventApproval); len(evs) != 0 {
		t.Errorf("%d approval row(s) recorded for a stopped loop's checkpoint, want none", len(evs))
	}
	assertStoppedRow(t, storedLoop(t, svc.store, run.LoopID), "after the refused approval")
}

// (4) A loop started with --watch, idle between events, is stopped and ends.
//
// The objective is declared the way POST /loops with watch_mode declares it
// (internal/api/handler/loop.go, declareWatch), and the loop is started the
// way that handler then starts it. The wait for the next event is what is
// left once Run's loop has gone through its iteration.
func TestStopReachesAWatchModeLoopWaitingForAnEvent(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()

	obj := req.Objective
	obj.Mode = objective.ModeStanding
	obj.Cadence = &objective.Cadence{Sense: "30s"}
	obj.Autonomy = &objective.Autonomy{Level: objective.AutonomySense, Ceiling: objective.AutonomySense}
	if err := svc.store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	req.Objective = obj
	req.MaxIter = 1

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	env.waitStarted(t)
	waitStoredCompleted(t, svc.store, run.LoopID)

	got, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got.Found != StopFoundRunning {
		t.Fatalf("Stop on a watch-mode loop waiting for its next event found it %q, want %q. "+
			"The loop service cannot reach this wait today: POST /loops with watch_mode declares the "+
			"objective standing at sense autonomy (internal/api/handler/loop.go, declareWatch) and Run's "+
			"one-shot loop ends, so the waiting between events is reconcile's tick, not a loop the service holds",
			got.Found, StopFoundRunning)
	}
	assertStoppedRow(t, storedLoop(t, svc.store, run.LoopID), "after stopping the watch")
}

// (5) Stopping a loop that has already ended changes nothing and says so,
// whether this process ran it or only the store remembers it. An ID nobody
// knows is said to be unknown.
func TestStoppingAnEndedLoopChangesNothingAndSaysSo(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()
	req.MaxIter = 1

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	before := waitStoredCompleted(t, svc.store, run.LoopID)
	time.Sleep(stopSettle)
	before = storedLoop(t, svc.store, run.LoopID)

	got, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop on an ended loop: %v", err)
	}
	if got.LoopID != run.LoopID || got.Found != StopFoundEnded {
		t.Errorf("Stop on an ended loop = %+v, want loop %s found %q", got, run.LoopID, StopFoundEnded)
	}
	assertRowUnchanged(t, before, storedLoop(t, svc.store, run.LoopID), "after stopping it in the process that ran it")

	next := restarted(t, svc, newStopEnv(false))
	got, err = next.Stop(ctx, run.LoopID, stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop on an ended loop after a restart: %v", err)
	}
	if got.Found != StopFoundEnded {
		t.Errorf("after a restart, Stop found an ended loop %q, want %q: its stored row says it ended", got.Found, StopFoundEnded)
	}
	assertRowUnchanged(t, before, storedLoop(t, svc.store, run.LoopID), "after stopping it from a new process")

	got, err = svc.Stop(ctx, "no-such-loop", stopPrincipal, stopReason)
	if err != nil {
		t.Fatalf("Stop on an unknown loop: %v", err)
	}
	if got.LoopID != "no-such-loop" || got.Found != StopFoundUnknown {
		t.Errorf("Stop on an unknown loop = %+v, want found %q", got, StopFoundUnknown)
	}
	if _, err := svc.store.GetLoopState(ctx, "no-such-loop"); err == nil {
		t.Error("stopping an unknown loop wrote a row for it")
	}
}

func assertRowUnchanged(t *testing.T, before, after coreloop.State, when string) {
	t.Helper()
	before.UpdatedAt, after.UpdatedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("%s: the stored row changed\nbefore: %+v\nafter:  %+v", when, before, after)
	}
}

// (6) A stop is not a failure. A run's failure is reported as status failed
// on the loop and on a one-shot objective, outcome errored, the error's text
// in Error (which reconcile reads), and an objective.failed event. A stopped
// loop shows none of these, and it does not read as still active either.
func TestAStoppedLoopDoesNotReadAsFailed(t *testing.T) {
	env := newStopEnv(true)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()

	events, unsubscribe := svc.hub.Subscribe(ctx, "obj:"+string(req.Objective.ID))
	defer unsubscribe()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	env.waitStarted(t)
	if _, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	env.waitCancelled(t)
	time.Sleep(stopSettle)

	st := storedLoop(t, svc.store, run.LoopID)
	if st.Status == objective.StatusFailed {
		t.Errorf("stored loop status = %q; a stop is not a failure", st.Status)
	}
	if st.Status == objective.StatusActive || st.Status == "" {
		t.Errorf("stored loop status = %q; a stopped loop has ended", st.Status)
	}
	if st.Outcome == coreloop.RunErrored {
		t.Errorf("stored outcome = %q; a stop is not an error", st.Outcome)
	}
	if st.Error != "" {
		t.Errorf("stored error = %q; reconcile reads this field as the run's failure, and a stop has none", st.Error)
	}
	if got, err := svc.store.GetObjective(ctx, req.Objective.ID); err != nil {
		t.Errorf("get objective: %v", err)
	} else if got.Status == objective.StatusFailed {
		t.Errorf("objective status = %q after its loop was stopped; a stop is not a failure", got.Status)
	}

	for {
		select {
		case e := <-events:
			if e.Type == event.TypeObjectiveFailed {
				t.Errorf("a stopped loop published %s with payload %v", e.Type, e.Payload)
			}
			continue
		default:
		}
		break
	}
}
