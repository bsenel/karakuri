package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
)

// rateLimitProvider is traceProvider that refuses its first len(errs) calls
// with errs in order, then plans reply. The agent runtime hands a provider's
// error back unwrapped, so the loop sees exactly what the adapter returned.
type rateLimitProvider struct {
	traceProvider
	errs  []error
	calls int
}

func (p *rateLimitProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	p.calls++
	if p.calls <= len(p.errs) {
		return llm.CompletionResponse{}, p.errs[p.calls-1]
	}
	return p.traceProvider.Complete(ctx, req)
}

// countingEnv is traceEnv that counts the actions it was asked to take.
type countingEnv struct {
	traceEnv
	mu   sync.Mutex
	acts int
}

func (e *countingEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	e.acts++
	e.mu.Unlock()
	return e.traceEnv.Act(ctx, a)
}

func (e *countingEnv) actCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.acts
}

// rateLimitLoopFixture is traceLoopFixture with the scripted provider swapped
// for rateLimitProvider, the environment swapped for countingEnv, and the
// retry wait recorded instead of slept.
func rateLimitLoopFixture(t *testing.T, errs ...error) (*serviceImpl, objective.Objective, *rateLimitProvider, *countingEnv, *[]time.Duration) {
	t.Helper()
	svc, _, obj := traceLoopFixture(t)

	provider := &rateLimitProvider{traceProvider: traceProvider{reply: tracePlan}, errs: errs}
	providers := llm.NewRegistry(nil)
	providers.Register(provider)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	env := &countingEnv{traceEnv: traceEnv{id: "test.env.ok"}}
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  "test.env.ok",
		Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build:  func(environment.BuildContext) (environment.Environment, error) { return env, nil },
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	waits := &[]time.Duration{}
	svc.sleep = func(_ context.Context, d time.Duration) { *waits = append(*waits, d) }
	return svc, obj, provider, env, waits
}

// rateLimitBounds act without approval on a confident plan and escalate on an
// unconfident one, so the only checkpoint the loop can raise is the
// confidence checkpoint an empty plan trips.
func rateLimitBounds() coreagent.AuthorityBounds {
	return coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions, ConfidenceThreshold: 0.7}
}

// drain collects what the hub delivered on ch, which runSync has finished
// publishing to by the time it is called.
func drain(ch <-chan event.Event) []event.Event {
	var out []event.Event
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

func stateOf(t *testing.T, svc *serviceImpl, loopID string) *loopState {
	t.Helper()
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	st := svc.states[loopID]
	if st == nil {
		t.Fatalf("no state for %s", loopID)
	}
	return st
}

func iterationSteps(its []loop.Iteration, step loop.Step) []loop.Iteration {
	var out []loop.Iteration
	for _, it := range its {
		if it.Step == step {
			out = append(out, it)
		}
	}
	return out
}

// A provider that refuses twice has given the loop no plan. The whole loop,
// not only stepReason, must end failed on the named limit: no confidence
// checkpoint raised from the empty plan, nothing acted on (Phase 36 Step 1
// and Acceptance in docs/roadmap.md).
func TestRunLoopEndsFailedOnRepeatedProviderRateLimit(t *testing.T) {
	const loopID = "loop-ratelimit-0001"
	svc, obj, provider, env, waits := rateLimitLoopFixture(t, rateLimited(), rateLimited(), rateLimited(), rateLimited())
	events, unsub := svc.hub.Subscribe(context.Background(), "obj:"+string(obj.ID))
	defer unsub()

	// A decision is queued so a loop that wrongly pauses resumes into a
	// rejection instead of hanging the test; a loop that never pauses leaves
	// it unread.
	reject := corecheckpoint.Decision{Choice: "reject", Approver: "ada"}
	runSync(svc, loopID, loop.Request{Objective: obj, Agent: traceAgent(rateLimitBounds()), MaxIter: 1}, &reject)

	if len(*waits) != 1 {
		t.Errorf("waited %d times, want exactly once", len(*waits))
	}
	if provider.calls != 2 {
		t.Errorf("agent called %d times, want 2: one retry, not a loop", provider.calls)
	}

	// No checkpoint: the queued decision was never consumed, nothing is
	// pending in the store, and no checkpoint event went out.
	st := stateOf(t, svc, loopID)
	if len(st.decisionCh) != 1 {
		t.Error("the loop paused and consumed the queued decision: a checkpoint was raised from a rate limit")
	}
	st.mu.RLock()
	paused, result := st.status.Paused, st.result
	st.mu.RUnlock()
	if paused {
		t.Error("loop status is Paused after a rate limit")
	}
	if result.CheckpointID != nil {
		t.Errorf("result carries checkpoint %q after a rate limit", *result.CheckpointID)
	}
	pending, err := svc.store.ListPendingCheckpoints(context.Background(), "")
	if err != nil {
		t.Fatalf("list pending checkpoints: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("%d checkpoints in the store after a rate limit, want none: %+v", len(pending), pending)
	}
	all := drain(events)
	for _, e := range all {
		if e.Type == event.TypeCheckpoint {
			t.Errorf("checkpoint event published after a rate limit: %+v", e.Payload)
		}
	}

	// No reason.plan placeholder anywhere in what the loop recorded.
	for _, it := range result.Iterations {
		if strings.Contains(fmt.Sprint(it.Output), "reason.plan") {
			t.Errorf("iteration %d %s output carries a reason.plan action: %v", it.Number, it.Step, it.Output)
		}
	}

	// Nothing acted on.
	if acts := iterationSteps(result.Iterations, loop.StepAct); len(acts) != 0 {
		t.Errorf("recorded %d act iterations after a rate limit, want none", len(acts))
	}
	if n := env.actCount(); n != 0 {
		t.Errorf("environment Act called %d times after a rate limit, want none", n)
	}

	// Ended failed through finalizeLoop, criteria not met.
	if result.Status != objective.StatusFailed {
		t.Errorf("result status = %q, want %q", result.Status, objective.StatusFailed)
	}
	persisted, err := svc.store.GetLoopState(context.Background(), loopID)
	if err != nil {
		t.Fatalf("get loop state: %v", err)
	}
	if !persisted.Completed || persisted.Status != objective.StatusFailed {
		t.Errorf("persisted state completed=%v status=%q, want a completed failure", persisted.Completed, persisted.Status)
	}
	// Reconcile reads the row, not the error value (Phase 36 step 3), so the
	// row has to say the provider refused.
	if !strings.Contains(persisted.Error, "claude") || !strings.Contains(persisted.Error, coreerrors.ErrProviderRateLimited.Error()) {
		t.Errorf("persisted error = %q, want the provider and the limit", persisted.Error)
	}
	stored, err := svc.store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	if stored.Status != objective.StatusFailed {
		t.Errorf("objective status = %q, want %q", stored.Status, objective.StatusFailed)
	}

	// The terminal event is where finalizeLoop reports the run to anyone
	// watching. It must say the criteria were not met and carry the error
	// that ended the run: the provider and its limit, as errors.Is reads it
	// when kept as an error, and by its text when kept as text.
	var failed *event.Event
	for i := range all {
		if all[i].Type == event.TypeObjectiveFailed {
			failed = &all[i]
		}
	}
	if failed == nil {
		t.Fatalf("no %s event among %d published", event.TypeObjectiveFailed, len(all))
	}
	if met, _ := failed.Payload["criteria_met"].(bool); met {
		t.Error("objective failed event says criteria_met = true")
	}
	switch runErr := failed.Payload["error"].(type) {
	case error:
		if !errors.Is(runErr, coreerrors.ErrProviderRateLimited) {
			t.Errorf("run error %v is not ErrProviderRateLimited", runErr)
		}
		if msg := runErr.Error(); !strings.Contains(msg, "claude") || !strings.Contains(msg, "rate limit") {
			t.Errorf("run error = %q, want it to name the provider and the limit", msg)
		}
	case string:
		if !strings.Contains(runErr, "claude") || !strings.Contains(runErr, "rate limit") {
			t.Errorf("run error = %q, want it to name the provider and the limit", runErr)
		}
	default:
		t.Errorf("objective failed event carries no run error (payload %v); finalizeLoop dropped the rate limit", failed.Payload)
	}
}

// One refusal and then a plan is an ordinary iteration: the retry's plan goes
// through decide and act as if the provider had never refused.
func TestRunLoopProceedsAfterOneProviderRateLimit(t *testing.T) {
	const loopID = "loop-ratelimit-0002"
	svc, obj, provider, env, waits := rateLimitLoopFixture(t, rateLimited())
	events, unsub := svc.hub.Subscribe(context.Background(), "obj:"+string(obj.ID))
	defer unsub()

	runSync(svc, loopID, loop.Request{Objective: obj, Agent: traceAgent(rateLimitBounds()), MaxIter: 1}, nil)

	if len(*waits) != 1 {
		t.Errorf("waited %d times, want exactly once", len(*waits))
	}
	if provider.calls != 2 {
		t.Errorf("agent called %d times, want 2", provider.calls)
	}

	st := stateOf(t, svc, loopID)
	st.mu.RLock()
	result := st.result
	st.mu.RUnlock()
	if result.CheckpointID != nil {
		t.Errorf("a confident plan raised checkpoint %q", *result.CheckpointID)
	}
	if acts := iterationSteps(result.Iterations, loop.StepAct); len(acts) != 1 {
		t.Fatalf("recorded %d act iterations, want the retry's plan acted on once", len(acts))
	}
	if n := env.actCount(); n != 1 {
		t.Errorf("environment Act called %d times, want 1 for the plan's one action", n)
	}

	for _, e := range drain(events) {
		if e.Type == event.TypeCheckpoint {
			t.Errorf("checkpoint event published for a confident plan: %+v", e.Payload)
		}
		if e.Type != event.TypeObjectiveFailed && e.Type != event.TypeObjectiveCompleted {
			continue
		}
		if runErr, ok := e.Payload["error"].(error); ok && errors.Is(runErr, coreerrors.ErrProviderRateLimited) {
			t.Errorf("loop ended on a rate limit it recovered from: %v", runErr)
		}
		if s, ok := e.Payload["error"].(string); ok && strings.Contains(s, "rate limit") {
			t.Errorf("loop ended on a rate limit it recovered from: %s", s)
		}
	}
}
