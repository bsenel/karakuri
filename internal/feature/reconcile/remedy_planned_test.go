package reconcile

// Phase 40 step 14b, review Blocker 2 on pull request #284: nothing in
// production set State.Remedy, so no remedy was ever stored or recalled. The
// tests in remedy_memory_test.go pass because the fake loop writes the remedy
// onto its final state by hand. These run a real loop whose planner names the
// remedy in its plan, and follow it from that answer to a stored memory and
// back into the next blocker's request. The remedy is text kept for recall
// only: it never changes the bounds, the tools or the budget of a pass.

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

const plannerRemedy = "shorten the observation the planner is given, then plan again"

// planWithRemedy is the planner's answer: the remedy it says the plan applies
// to the blocker it was given, and no action, so the pass ends without asking
// a person and its reading can confirm the blocker is gone.
const planWithRemedy = `{"actions":[],` +
	`"confidence":0.95,"reasoning":"the plan errored on an oversized observation","remedy":"` + plannerRemedy + `"}`

// remedyPlanner answers every call with planWithRemedy. It is registered as
// "claude", the provider an agent with no preference is given.
type remedyPlanner struct{ llm.ProviderAdapter }

func (remedyPlanner) Name() string                   { return "claude" }
func (remedyPlanner) Available(context.Context) bool { return true }
func (remedyPlanner) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: planWithRemedy, TokensUsed: 10}, nil
}
func (remedyPlanner) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: planWithRemedy, Done: true}
	close(ch)
	return ch, nil
}

// okEnv serves test.act.ok and answers every action.
type okEnv struct{}

func (okEnv) ID() environment.EnvironmentID { return "test.env.ok" }
func (okEnv) Domain() string                { return "test" }
func (okEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: "test.env.ok", Version: "v1", Timestamp: time.Now().UTC()}, nil
}
func (okEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: true}, nil
}
func (okEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}
func (okEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: "test.env.ok"}, nil
}

// realLoops is the production loop service over the fixture's store, with the
// requests reconcile gave it recorded.
type realLoops struct {
	LoopRunner
	mu   sync.Mutex
	reqs []coreloop.Request
}

func (r *realLoops) Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
	return r.LoopRunner.Run(ctx, req)
}

func (r *realLoops) last(t *testing.T) coreloop.Request {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		t.Fatal("the real loop was never started")
	}
	return r.reqs[len(r.reqs)-1]
}

func useRealLoop(t *testing.T, f *fixture) *realLoops {
	t.Helper()
	hub := event.NewHub()
	otel := observability.NewOTel(nil)
	providers := llm.NewRegistry(nil)
	providers.Register(remedyPlanner{})
	envReg := environment.NewRegistry()
	if err := envReg.Register(environment.Factory{
		EnvID: "test.env.ok", Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build:  func(environment.BuildContext) (environment.Environment, error) { return okEnv{}, nil },
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := featureloop.NewService(f.store, platformagent.NewFactory(providers, hub, otel, nil), nil, envReg,
		featurememory.NewService(f.store, 5), featurecp.NewService(f.store, hub), nil, nil, hub, otel, nil,
		karakuriquota.Deps{}, gateway.New(f.store, karakuriquota.Deps{}, nil), nil, 0)
	r := &realLoops{LoopRunner: svc}
	f.svc.loops = r
	return r
}

// remedyStream is a stream holding a plan_error blocker whose next pass runs
// the real loop once. The blocker clears when that pass does not error.
func remedyStream(t *testing.T) (*fixture, objective.Objective, *realLoops) {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:       &objective.Cadence{Every: "1h"},
		Autonomy:      &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
		MaxIterations: 1,
	})
	holdBlocker(t, f, obj, blk(reconcile.BlockerPlanError, string(obj.ID), 2, t0))
	return f, obj, useRealLoop(t, f)
}

// (b) The planner names a remedy, the pass's reading confirms it (the
// plan_error blocker it was given is gone), and it is stored once with the
// blocker's kind; the next plan_error blocker recalls it and another kind
// does not.
func TestARemedyThePlannerNamesIsKeptAndRecalledByItsKind(t *testing.T) {
	f, obj, _ := remedyStream(t)

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if st := f.state(t, obj.ID); len(st.Blockers) != 0 {
		t.Fatalf("the pass did not clear its plan_error blocker, so it confirms nothing: %+v", st.Blockers)
	}
	got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerPlanError)
	if len(got) != 1 {
		t.Fatalf("stored %d remedies of kind plan_error, want 1: the remedy the planner named was not kept: %+v", len(got), got)
	}
	if got[0].Content != plannerRemedy {
		t.Fatalf("stored remedy = %q, want the planner's %q", got[0].Content, plannerRemedy)
	}

	// The next blocker of the same kind is handed it; the fake loop answers
	// from here so the request is all that is read.
	f.clock = f.clock.Add(2 * time.Hour)
	same := unblockRequest(t, f, obj, blk(reconcile.BlockerPlanError, string(obj.ID), 1, t0)).Unblock
	if same == nil || len(same.Recalled) != 1 || same.Recalled[0] != plannerRemedy {
		t.Fatalf("same kind: Unblock = %+v, want the planner's remedy recalled", same)
	}
	f.clock = f.clock.Add(2 * time.Hour)
	other := unblockRequest(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 1, t0)).Unblock
	if other == nil {
		t.Fatal("other kind: request.Unblock is nil")
	}
	if len(other.Recalled) != 0 {
		t.Fatalf("other kind: Recalled = %q, want nothing", other.Recalled)
	}
}

// (c) A recalled remedy is text, not authority: the pass that recalls it is
// given the same bounds, agent, rung and budget as the pass that named it.
func TestARecalledRemedyLeavesThePassesLimitsAsTheyWere(t *testing.T) {
	f, obj, real := remedyStream(t)
	f.pass(t, obj.ID, reconcile.TriggerManual)
	before := real.last(t)

	f.clock = f.clock.Add(2 * time.Hour)
	after := unblockRequest(t, f, obj, blk(reconcile.BlockerPlanError, string(obj.ID), 1, t0))
	if after.Unblock == nil || len(after.Unblock.Recalled) != 1 || after.Unblock.Recalled[0] != plannerRemedy {
		t.Fatalf("Unblock = %+v, want the planner's remedy recalled before its limits are compared", after.Unblock)
	}

	if !reflect.DeepEqual(after.Agent.Authority, before.Agent.Authority) {
		t.Errorf("AuthorityBounds changed with a recalled remedy:\n got %+v\nwant %+v", after.Agent.Authority, before.Agent.Authority)
	}
	if !reflect.DeepEqual(after.Agent.Capabilities, before.Agent.Capabilities) {
		t.Errorf("allowed capabilities changed with a recalled remedy: got %v, want %v", after.Agent.Capabilities, before.Agent.Capabilities)
	}
	if after.Agent.ID != before.Agent.ID {
		t.Errorf("agent = %q, want %q", after.Agent.ID, before.Agent.ID)
	}
	if after.MaxIter != before.MaxIter {
		t.Errorf("MaxIter = %d, want %d: a remedy does not buy a pass more iterations", after.MaxIter, before.MaxIter)
	}
	if after.AutonomyRung != before.AutonomyRung {
		t.Errorf("AutonomyRung = %q, want %q", after.AutonomyRung, before.AutonomyRung)
	}
	// And the limits are the stream's own, not something the remedy wrote.
	if got := f.loops.bounds(); !reflect.DeepEqual(got, before.Agent.Authority) {
		t.Errorf("the loop ran with bounds %+v, want the stream's %+v", got, before.Agent.Authority)
	}
}
