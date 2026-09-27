package loop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/telemetry"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
)

// recordedSpan is what recordingTracer kept of one span. parent is the id of
// the span the ctx passed to Start carried, and zero for a root.
type recordedSpan struct {
	id     int
	name   string
	attrs  map[string]any
	parent int
	ended  bool
	err    string
}

// traceSpanKey is private to the fake, so the only way a span becomes a child
// is by being started from a ctx this tracer returned.
type traceSpanKey struct{}

// recordingTracer is a telemetry.Tracer that remembers every span. Guarded by
// a mutex because the loop is free to act from more than one goroutine, and a
// fake that raced would fail these tests for the wrong reason.
type recordingTracer struct {
	mu    sync.Mutex
	spans []*recordedSpan
}

func (t *recordingTracer) Start(ctx context.Context, name string, attrs ...telemetry.Attribute) (context.Context, telemetry.Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, _ := ctx.Value(traceSpanKey{}).(int)
	s := &recordedSpan{
		id:     len(t.spans) + 1,
		name:   name,
		attrs:  map[string]any{},
		parent: parent,
	}
	for _, a := range attrs {
		s.attrs[a.Key] = a.Value
	}
	t.spans = append(t.spans, s)
	return context.WithValue(ctx, traceSpanKey{}, s.id), &recordingSpan{t: t, s: s}
}

// snapshot copies what was recorded, so assertions never read a span the loop
// might still be writing.
func (t *recordingTracer) snapshot() []recordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]recordedSpan, 0, len(t.spans))
	for _, s := range t.spans {
		c := *s
		c.attrs = make(map[string]any, len(s.attrs))
		for k, v := range s.attrs {
			c.attrs[k] = v
		}
		out = append(out, c)
	}
	return out
}

func (t *recordingTracer) named(prefix string) []recordedSpan {
	var out []recordedSpan
	for _, s := range t.snapshot() {
		if strings.HasPrefix(s.name, prefix) {
			out = append(out, s)
		}
	}
	return out
}

type recordingSpan struct {
	t *recordingTracer
	s *recordedSpan
}

func (r *recordingSpan) SetAttributes(attrs ...telemetry.Attribute) {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	for _, a := range attrs {
		r.s.attrs[a.Key] = a.Value
	}
}

func (r *recordingSpan) SetError(msg string) {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	r.s.err = msg
}

func (r *recordingSpan) End() {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	r.s.ended = true
}

// traceEnv succeeds at everything unless told to fail its actions, and then
// fails them the way an adapter does — with a Go error, not a result.
type traceEnv struct {
	id      environment.EnvironmentID
	failAct bool
}

func (e *traceEnv) ID() environment.EnvironmentID { return e.id }
func (e *traceEnv) Domain() string                { return "test" }

func (e *traceEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: e.id, Version: string(e.id) + "-v1", Timestamp: time.Now().UTC()}, nil
}

func (e *traceEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	if e.failAct {
		return environment.ActionResult{}, errors.New("adapter refused the call")
	}
	return environment.ActionResult{Success: true}, nil
}

func (e *traceEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (e *traceEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}

func attrOf(s recordedSpan, key string) any { return s.attrs[key] }

// Every call to an environment is a tool call, and gets exactly one span.
// The unrouted action never reached an environment, so it has nothing to
// wrap: a span for it would claim a tool ran when none did.
func TestStepActOpensOneExecuteToolSpanPerEnvironmentCall(t *testing.T) {
	sc := observeFixture(t,
		&traceEnv{id: "test.env.ok"},
		&traceEnv{id: "test.env.broken", failAct: true},
	)
	tr := &recordingTracer{}
	sc.svc.tracer = tr

	ctx, root := tr.Start(context.Background(), "root")
	stepAct(ctx, sc, plan{
		Confidence: 0.9,
		Actions: []plannedAction{
			{CapabilityID: "test.act.ok", EnvID: "test.env.ok"},
			{CapabilityID: "test.act.broken", EnvID: "test.env.broken"},
			{CapabilityID: "test.act.nowhere", EnvID: "test.env.missing"},
		},
	})
	root.End()

	all := tr.snapshot()
	if len(all) != 3 {
		names := make([]string, 0, len(all))
		for _, s := range all {
			names = append(names, s.name)
		}
		t.Fatalf("recorded %d spans %v, want the root and one per routed action (3)", len(all), names)
	}
	rootID := all[0].id

	tools := tr.named("execute_tool ")
	if len(tools) != 2 {
		t.Fatalf("execute_tool spans = %d, want 2", len(tools))
	}

	want := map[string]struct {
		env    string
		failed bool
	}{
		"test.act.ok":     {env: "test.env.ok"},
		"test.act.broken": {env: "test.env.broken", failed: true},
	}
	for _, s := range tools {
		capID := strings.TrimPrefix(s.name, "execute_tool ")
		w, ok := want[capID]
		if !ok {
			t.Errorf("unexpected span %q", s.name)
			continue
		}
		delete(want, capID)

		if got := attrOf(s, telemetry.GenAIOperationName); got != telemetry.OpExecuteTool {
			t.Errorf("%s: %s = %v, want %q", s.name, telemetry.GenAIOperationName, got, telemetry.OpExecuteTool)
		}
		if got := attrOf(s, telemetry.GenAIToolName); got != capID {
			t.Errorf("%s: %s = %v, want %q", s.name, telemetry.GenAIToolName, got, capID)
		}
		if got := attrOf(s, telemetry.AttrKarakuriEnvironmentID); got != w.env {
			t.Errorf("%s: %s = %v, want %q", s.name, telemetry.AttrKarakuriEnvironmentID, got, w.env)
		}
		if w.failed && s.err == "" {
			t.Errorf("%s: the action failed and the span carries no error", s.name)
		}
		if !w.failed && s.err != "" {
			t.Errorf("%s: the action succeeded and the span says %q", s.name, s.err)
		}
		if s.parent != rootID {
			t.Errorf("%s: parent = %d, want the span in the ctx stepAct was given (%d)", s.name, s.parent, rootID)
		}
		if !s.ended {
			t.Errorf("%s was never ended", s.name)
		}
	}
	for capID := range want {
		t.Errorf("no execute_tool span for %s", capID)
	}
}

// traceProvider is an LLM that always plans the same thing. It embeds the
// interface for AsLLM, which the agent runtime never calls on this path, so
// the test needs no vendor import to satisfy it.
type traceProvider struct {
	llm.ProviderAdapter
	reply string
}

func (p *traceProvider) Name() string                   { return "trace" }
func (p *traceProvider) Available(context.Context) bool { return true }

func (p *traceProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: p.reply, TokensUsed: 10}, nil
}

func (p *traceProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: p.reply, Done: true}
	close(ch)
	return ch, nil
}

const tracePlan = `{"actions":[{"capability":"test.act.ok","env_id":"test.env.ok","reason":"r"}],"confidence":0.95,"reasoning":"r"}`

// traceLoopFixture is a service that can run the whole loop: a real agent
// factory over a scripted provider, one environment serving one capability,
// and an objective with no success criteria so it runs every iteration it is
// given.
func traceLoopFixture(t *testing.T) (*serviceImpl, *recordingTracer, objective.Objective) {
	t.Helper()
	svc, _, store := newResumeFixture(t)
	svc.otel = observability.NewOTel(nil)
	svc.memSvc = featurememory.NewService(store, 5)

	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: tracePlan})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  "test.env.ok",
		Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return &traceEnv{id: "test.env.ok"}, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	tr := &recordingTracer{}
	svc.tracer = tr

	obj := objective.Objective{ID: "obj-trace", Title: "trace the loop", Domain: "test"}
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return svc, tr, obj
}

func traceAgent(bounds coreagent.AuthorityBounds) coreagent.Definition {
	return coreagent.Definition{
		ID:                "trace-agent",
		Name:              "Trace Agent",
		Domain:            "test",
		ReasoningStrategy: coreagent.ReasoningChainOfThought,
		Authority:         bounds,
		LLMHints:          capability.LLMHints{PreferredProvider: "trace"},
	}
}

// runSync registers a loop the way Run does and drives it on this goroutine,
// so the test reads the spans after the loop has returned rather than racing
// it.
func runSync(svc *serviceImpl, loopID string, req loop.Request, pending *corecheckpoint.Decision) {
	state := &loopState{
		id:         loopID,
		decisionCh: make(chan corecheckpoint.Decision, 1),
		request:    req,
		result:     loop.Result{LoopID: loopID, ObjectiveID: req.Objective.ID, Status: objective.StatusActive},
		status:     loop.Status{LoopID: loopID, ObjectiveID: req.Objective.ID, Step: loop.StepObserve},
	}
	if pending != nil {
		state.decisionCh <- *pending
	}
	svc.mu.Lock()
	svc.states[loopID] = state
	svc.mu.Unlock()
	svc.runLoop(context.Background(), loopID, req)
}

// One invoke_agent span per iteration, each a root, each ended, and every tool
// call inside an iteration nested beneath it. A trace viewer groups by root,
// so an execute_tool span with no invoke_agent parent is a tool call nobody
// can attribute to an iteration.
func TestRunLoopOpensOneInvokeAgentSpanPerIteration(t *testing.T) {
	const n = 3
	svc, tr, obj := traceLoopFixture(t)
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})

	runSync(svc, "loop-trace-0001", loop.Request{Objective: obj, Agent: def, MaxIter: n}, nil)

	agents := tr.named("invoke_agent ")
	if len(agents) != n {
		t.Fatalf("invoke_agent spans = %d, want one per iteration (%d)", len(agents), n)
	}
	iterationSpans := map[int]bool{}
	for _, s := range agents {
		iterationSpans[s.id] = true
		if s.name != "invoke_agent "+def.Name {
			t.Errorf("span named %q, want %q", s.name, "invoke_agent "+def.Name)
		}
		if got := attrOf(s, telemetry.GenAIOperationName); got != telemetry.OpInvokeAgent {
			t.Errorf("%s = %v, want %q", telemetry.GenAIOperationName, got, telemetry.OpInvokeAgent)
		}
		if got := attrOf(s, telemetry.GenAIAgentName); got != def.Name {
			t.Errorf("%s = %v, want %q", telemetry.GenAIAgentName, got, def.Name)
		}
		if !s.ended {
			t.Errorf("invoke_agent span %d was never ended", s.id)
		}
		if s.parent != 0 {
			t.Errorf("invoke_agent span %d has parent %d; an iteration is a root", s.id, s.parent)
		}
	}

	tools := tr.named("execute_tool ")
	if len(tools) != n {
		t.Errorf("execute_tool spans = %d, want one per iteration's single action (%d)", len(tools), n)
	}
	for _, s := range tools {
		if !iterationSpans[s.parent] {
			t.Errorf("%s (span %d) has parent %d, which is not an invoke_agent span", s.name, s.id, s.parent)
		}
	}
}

// An iteration that ends at a rejected checkpoint returns from the middle of
// runLoop. Its span must still be ended, or the trace holds an iteration that
// never finishes.
func TestRejectedCheckpointStillEndsTheInvokeAgentSpan(t *testing.T) {
	svc, tr, obj := traceLoopFixture(t)
	// Zero autonomous actions: every non-empty plan escalates.
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	reject := corecheckpoint.Decision{Choice: "reject", Approver: "ada"}

	runSync(svc, "loop-trace-0002", loop.Request{Objective: obj, Agent: def, MaxIter: 3}, &reject)

	agents := tr.named("invoke_agent ")
	if len(agents) != 1 {
		t.Fatalf("invoke_agent spans = %d, want 1 — the loop stops at the first rejection", len(agents))
	}
	if !agents[0].ended {
		t.Error("the rejected iteration's invoke_agent span was never ended")
	}
	if tools := tr.named("execute_tool "); len(tools) != 0 {
		t.Errorf("execute_tool spans = %d after a rejection, want 0 — nothing was acted on", len(tools))
	}
}
