package loop

// Phase 40 step 5, part A: an approval belongs to the plan it approved.
//
// What the code does today, read before writing these:
//   - stepDecide (decide.go) escalates by creating a checkpoint whose Actions
//     are the draft the reviewer is shown. Nothing on the checkpoint names the
//     plan as such, and the plan itself lives only in runLoop's local p.
//   - ResumeStoredLoops (service.go) does not replay a paused row at boot: it
//     registers it and blocks a waiter on the decision channel. On a decision
//     the waiter puts the decision back and calls runLoop from the top, which
//     observes and reasons again, so the plan that runs is the one the replay
//     re-derives, not the one on the checkpoint. The waiter sets
//     loopState.answered, so the replay's first stepDecide (takeAnswered,
//     decide.go) raises no second checkpoint and reads the decision waiting on
//     the channel as the answer to the old one: an approval of one plan then
//     runs another.
//
// The step asks that a restored loop waiting at a checkpoint keep waiting on
// that checkpoint, make no new plan, and, once approved, run exactly the
// actions that checkpoint showed.

import (
	"context"
	"encoding/json"
	"reflect"
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
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const (
	// shownPlan is what the reasoner planned before the restart, and so what
	// the checkpoint showed the reviewer.
	shownPlan = `{"actions":[{"capability":"test.act.ok","env_id":"test.env.ok","params":{"target":"shown.txt"},"reason":"the plan shown"}],"confidence":0.95,"reasoning":"r"}`
	// replannedPlan is what the reasoner would plan after the restart. If any
	// of it runs, the approval authorised a plan nobody saw.
	replannedPlan = `{"actions":[{"capability":"test.act.other","env_id":"test.env.ok","params":{"target":"replanned.txt"},"reason":"a different plan"}],"confidence":0.95,"reasoning":"r"}`
)

// countingProvider plans the same thing every time and counts how often it
// was asked, which is how many reason steps ran.
type countingProvider struct {
	traceProvider
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *countingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.traceProvider.Complete(ctx, req)
}

func (p *countingProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.traceProvider.Stream(ctx, req)
}

// ranAction is one action an environment was asked to take, with how many
// reason steps had run by then.
type ranAction struct {
	capability    string
	params        map[string]any
	reasonsBefore int
}

// planEnv serves both capabilities and records every action it is asked to
// take.
type planEnv struct {
	traceEnv
	provider *countingProvider

	mu  sync.Mutex
	ran []ranAction
}

func (e *planEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ran = append(e.ran, ranAction{
		capability:    string(a.CapabilityID),
		params:        a.Params,
		reasonsBefore: e.provider.count(),
	})
	return environment.ActionResult{Success: true}, nil
}

func (e *planEnv) actions() []ranAction {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ranAction(nil), e.ran...)
}

// planProcess is one server process: a reasoner that always plans reply, and
// an environment that records what it runs.
func planProcess(t *testing.T, svc *serviceImpl, reply string) (*countingProvider, *planEnv) {
	t.Helper()
	provider := &countingProvider{traceProvider: traceProvider{reply: reply}}
	providers := llm.NewRegistry(nil)
	providers.Register(provider)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	env := &planEnv{traceEnv: traceEnv{id: "test.env.ok"}, provider: provider}
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  "test.env.ok",
		Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok", "test.act.other"},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return env, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg
	return provider, env
}

// approvalRestart is a loop paused at a checkpoint, stored, and a server
// restarted over the same store: a fresh service with nothing in memory, its
// own checkpoint service, and a reasoner that would now plan something else.
type approvalRestart struct {
	before   *serviceImpl
	next     *serviceImpl
	provider *countingProvider
	env      *planEnv
	loopID   string
	cp       corecheckpoint.Checkpoint
	obj      objective.Objective
}

func restartAtCheckpoint(t *testing.T) approvalRestart {
	t.Helper()
	before, _, obj := traceLoopFixture(t)
	before.tracer = nil
	planProcess(t, before, shownPlan)
	ctx := context.Background()

	// No action may be taken on the agent's own authority, so the first plan
	// escalates.
	run, err := before.Run(ctx, coreloop.Request{
		Objective: obj,
		Agent:     traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: 0}),
		MaxIter:   1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cpID := waitAtCheckpoint(t, before, run.LoopID)
	waitStoredPausedAt(t, before.store, run.LoopID, cpID)

	cp, err := before.store.GetCheckpoint(ctx, cpID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if len(cp.Actions) == 0 {
		t.Fatal("the checkpoint shows no actions; there is no plan to hold the approval to")
	}

	// The old process is gone. Its runner is still blocked on a channel only
	// its own service can reach, so nothing below can wake it.
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
	provider, env := planProcess(t, next, replannedPlan)

	if err := next.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	return approvalRestart{before: before, next: next, provider: provider, env: env, loopID: run.LoopID, cp: cp, obj: obj}
}

func waitStoredPausedAt(t *testing.T, store storage.StorageAdapter, loopID, cpID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := store.GetLoopState(context.Background(), loopID)
		if err == nil && st.Paused && st.CheckpointID == cpID {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the loop was never stored paused at %s: paused=%v checkpoint=%q (err %v)", cpID, st.Paused, st.CheckpointID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// checkpointsFor is every checkpoint the objective has, pending or resolved.
func checkpointsFor(t *testing.T, store storage.StorageAdapter, obj objective.Objective) []corecheckpoint.Checkpoint {
	t.Helper()
	out := pendingFor(t, store, obj)
	resolved, err := store.ListResolvedCheckpoints(context.Background(), storage.ResolvedCheckpointFilter{})
	if err != nil {
		t.Fatalf("list resolved: %v", err)
	}
	for _, cp := range resolved {
		if cp.ObjectiveID == obj.ID {
			out = append(out, cp)
		}
	}
	return out
}

func checkpointIDs(cps []corecheckpoint.Checkpoint) []string {
	ids := make([]string, 0, len(cps))
	for _, cp := range cps {
		ids = append(ids, cp.ID)
	}
	return ids
}

// normalised puts params through JSON, the way they were stored on the
// checkpoint, so a comparison asks about values and not Go types.
func normalised(t *testing.T, params map[string]any) map[string]any {
	t.Helper()
	if len(params) == 0 {
		return nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return out
}

// (1) A restored loop that was waiting at a checkpoint keeps waiting on it.
// No reason step runs and no second checkpoint is raised.
func TestApprovalBoundRestoredLoopKeepsWaitingWithoutReplanning(t *testing.T) {
	r := restartAtCheckpoint(t)

	// Long enough for a replay that was wrongly started to reason and raise
	// a checkpoint of its own.
	time.Sleep(stopSettle)

	if n := r.provider.count(); n != 0 {
		t.Errorf("the restored loop ran %d reason step(s) while it waited; a loop waiting at a checkpoint has no plan to make", n)
	}
	all := checkpointsFor(t, r.next.store, r.obj)
	if len(all) != 1 || all[0].ID != r.cp.ID {
		t.Errorf("checkpoints for the objective = %v, want only %s", checkpointIDs(all), r.cp.ID)
	}
	if got := storedLoop(t, r.next.store, r.loopID); !got.Paused || got.CheckpointID != r.cp.ID {
		t.Errorf("stored loop paused=%v checkpoint=%q, want paused at %s", got.Paused, got.CheckpointID, r.cp.ID)
	}
	if ran := r.env.actions(); len(ran) != 0 {
		t.Errorf("%d action(s) ran before anyone decided, want none", len(ran))
	}
}

// (2) Approving the checkpoint runs exactly the actions it showed, even though
// the reasoner would now plan something else, and no reason step runs before
// them.
func TestApprovalBoundApprovingRunsThePlanThatWasShown(t *testing.T) {
	r := restartAtCheckpoint(t)
	ctx := context.Background()

	approve := corecheckpoint.Decision{Choice: corecheckpoint.ChoiceApprove, Approver: "user:ada", Note: "run what I saw"}
	if err := r.next.cpSvc.Resolve(ctx, r.cp.ID, approve); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	waitStoredCompleted(t, r.next.store, r.loopID)

	ran := r.env.actions()
	if len(ran) != len(r.cp.Actions) {
		t.Fatalf("%d action(s) ran after the approval, want the %d the checkpoint showed: %+v", len(ran), len(r.cp.Actions), ran)
	}
	for i, want := range r.cp.Actions {
		got := ran[i]
		if got.capability != want.CapabilityID {
			t.Errorf("action %d ran %q, want %q as shown on the checkpoint", i, got.capability, want.CapabilityID)
		}
		if !reflect.DeepEqual(normalised(t, got.params), normalised(t, want.Params)) {
			t.Errorf("action %d ran with params %v, want %v as shown on the checkpoint", i, got.params, want.Params)
		}
		if got.reasonsBefore != 0 {
			t.Errorf("action %d ran after %d reason step(s); the approved plan needs none", i, got.reasonsBefore)
		}
	}

	// The approval answered the checkpoint that was shown, and raised no other.
	all := checkpointsFor(t, r.next.store, r.obj)
	if len(all) != 1 || all[0].ID != r.cp.ID {
		t.Errorf("checkpoints for the objective = %v, want only %s", checkpointIDs(all), r.cp.ID)
	}
	if left := pendingFor(t, r.next.store, r.obj); len(left) != 0 {
		t.Errorf("checkpoints left pending after the approval: %v", checkpointIDs(left))
	}
}

// (3) The checkpoint names the plan it was raised for, set by the runner when
// it escalates, and the stored loop names that checkpoint.
func TestApprovalBoundCheckpointNamesThePlanItWasRaisedFor(t *testing.T) {
	r := restartAtCheckpoint(t)

	if r.cp.PlanID == "" {
		t.Error("the checkpoint the runner raised names no plan; an approval of it cannot be held to the plan it showed")
	}
	if got := storedLoop(t, r.next.store, r.loopID); got.CheckpointID != r.cp.ID {
		t.Errorf("stored loop names checkpoint %q, want %s", got.CheckpointID, r.cp.ID)
	}

	r.next.mu.RLock()
	state := r.next.states[r.loopID]
	r.next.mu.RUnlock()
	if state == nil {
		t.Fatal("the restart did not register the paused loop")
	}
	state.mu.RLock()
	cpID := state.result.CheckpointID
	state.mu.RUnlock()
	if cpID == nil || *cpID != r.cp.ID {
		t.Errorf("the restored loop waits on checkpoint %v, want %s", cpID, r.cp.ID)
	}
}

// (4) A reject after the restart keeps today's behaviour: the replay reads it
// at its first decide, runs no action, raises no second checkpoint, and ends
// the loop.
func TestApprovalBoundRejectAfterRestartRunsNothing(t *testing.T) {
	r := restartAtCheckpoint(t)
	ctx := context.Background()

	reject := corecheckpoint.Decision{Choice: corecheckpoint.ChoiceReject, Approver: "user:ada", Note: "no"}
	if err := r.next.cpSvc.Resolve(ctx, r.cp.ID, reject); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	waitStoredCompleted(t, r.next.store, r.loopID)

	if ran := r.env.actions(); len(ran) != 0 {
		t.Errorf("%d action(s) ran after a reject, want none: %+v", len(ran), ran)
	}
	all := checkpointsFor(t, r.next.store, r.obj)
	if len(all) != 1 || all[0].ID != r.cp.ID {
		t.Errorf("checkpoints for the objective = %v, want only %s", checkpointIDs(all), r.cp.ID)
	}
}

// (5) A modify after the restart keeps today's behaviour: the replay reasons
// again and revises the plan it makes, and a revised plan still outside the
// bounds is shown on a new checkpoint that names its own plan. Nothing runs
// until that one is answered.
func TestApprovalBoundModifyAfterRestartReplansAndAsksAgain(t *testing.T) {
	r := restartAtCheckpoint(t)
	ctx := context.Background()

	modify := corecheckpoint.Decision{Choice: corecheckpoint.ChoiceModify, Approver: "user:ada", Note: "narrower"}
	if err := r.next.cpSvc.Resolve(ctx, r.cp.ID, modify); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var second corecheckpoint.Checkpoint
	deadline := time.Now().Add(stopBound)
	for second.ID == "" {
		for _, cp := range pendingFor(t, r.next.store, r.obj) {
			if cp.ID != r.cp.ID {
				second = cp
			}
		}
		if second.ID == "" && time.Now().After(deadline) {
			t.Fatalf("no checkpoint was raised for the modified plan within %s", stopBound)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if n := r.provider.count(); n == 0 {
		t.Error("the modify ran no reason step; a modify revises the plan the replay makes")
	}
	if second.PlanID == "" || second.PlanID == r.cp.PlanID {
		t.Errorf("the modified plan's checkpoint names plan %q, want one of its own (the first was %q)", second.PlanID, r.cp.PlanID)
	}
	if ran := r.env.actions(); len(ran) != 0 {
		t.Errorf("%d action(s) ran before the modified plan was answered, want none: %+v", len(ran), ran)
	}
}

// (6) A checkpoint that recorded no plan, as one written before checkpoints
// named theirs, has nothing to hold an approval to: the approval falls back
// to the replay, whose re-derived plan is what runs.
func TestApprovalBoundCheckpointWithoutAPlanFallsBackToReplay(t *testing.T) {
	r := restartAtCheckpoint(t)
	ctx := context.Background()

	old := r.cp
	old.PlanID = ""
	if err := r.next.store.SaveCheckpoint(ctx, old); err != nil {
		t.Fatalf("save checkpoint without a plan: %v", err)
	}

	approve := corecheckpoint.Decision{Choice: corecheckpoint.ChoiceApprove, Approver: "user:ada"}
	if err := r.next.cpSvc.Resolve(ctx, r.cp.ID, approve); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	waitStoredCompleted(t, r.next.store, r.loopID)

	ran := r.env.actions()
	if len(ran) != 1 || ran[0].capability != "test.act.other" || ran[0].reasonsBefore == 0 {
		t.Errorf("actions run = %+v, want the replay's one test.act.other after a reason step", ran)
	}
	all := checkpointsFor(t, r.next.store, r.obj)
	if len(all) != 1 || all[0].ID != r.cp.ID {
		t.Errorf("checkpoints for the objective = %v, want only %s", checkpointIDs(all), r.cp.ID)
	}
}
