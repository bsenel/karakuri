package loop

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// failingAgent is a model call that never comes back.
type failingAgent struct{}

func (failingAgent) Run(context.Context, coreagent.Input) (coreagent.Output, error) {
	return coreagent.Output{}, errors.New("provider unavailable")
}
func (failingAgent) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, nil
}

// The plan remembers who drafted it, so a decision made on it can say which
// model it came from.
func TestPlanKeepsTheProviderAndModelThatProducedIt(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: goodPlan, Confidence: 0.8, Provider: "anthropic", Model: "model-a"},
	}}

	p := stepReason(context.Background(), scriptedReasonContext(t, agent), loop.WorldState{})

	if p.provider != "anthropic" || p.model != "model-a" {
		t.Errorf("provider/model = %q/%q, want anthropic/model-a", p.provider, p.model)
	}
}

// When the retry is what parsed, the retry's call is the one that produced
// the plan. A fallback provider may have served it.
func TestPlanFromARetryNamesTheCallThatParsed(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "prose, not a plan", Provider: "anthropic", Model: "model-a"},
		{Content: goodPlan, Confidence: 0.8, Provider: "fallback", Model: "model-b"},
	}}

	p := stepReason(context.Background(), scriptedReasonContext(t, agent), loop.WorldState{})

	if p.provider != "fallback" || p.model != "model-b" {
		t.Errorf("provider/model = %q/%q, want fallback/model-b", p.provider, p.model)
	}
}

// A Reflexion revision replaces the draft, so it is the revising call that is
// named.
func TestRevisedPlanNamesTheCallThatRevisedIt(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: goodPlan, Confidence: 0.8, Provider: "anthropic", Model: "model-a"},
		{Content: "the weakest assumption is the test command", Provider: "anthropic", Model: "model-a"},
		{Content: goodPlan, Confidence: 0.9, Provider: "fallback", Model: "model-b"},
	}}
	sc := scriptedReasonContext(t, agent)
	sc.agentDef.ReasoningStrategy = coreagent.ReasoningReflexion

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if p.provider != "fallback" || p.model != "model-b" {
		t.Errorf("provider/model = %q/%q, want fallback/model-b", p.provider, p.model)
	}
}

// The placeholder built when the call fails was produced by no model. It
// says so by saying nothing, never by guessing a name.
func TestPlaceholderPlanAfterAFailedCallNamesNoModel(t *testing.T) {
	p := stepReason(context.Background(), scriptedReasonContext(t, failingAgent{}), loop.WorldState{})

	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "reason.plan" {
		t.Fatalf("expected the failure placeholder, got %+v", p.Actions)
	}
	if p.provider != "" || p.model != "" {
		t.Errorf("provider/model = %q/%q, want both empty", p.provider, p.model)
	}
}

// provenanceProvider is an LLM that names the model it serves, and can be told
// to fail every call. It embeds the interface for AsLLM, which the agent
// runtime never calls on this path.
type provenanceProvider struct {
	llm.ProviderAdapter
	name, model string
	fail        bool
}

func (p *provenanceProvider) Name() string                   { return p.name }
func (p *provenanceProvider) Model() string                  { return p.model }
func (p *provenanceProvider) Available(context.Context) bool { return true }

func (p *provenanceProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	if p.fail {
		return llm.CompletionResponse{}, errors.New("provider unavailable")
	}
	return llm.CompletionResponse{Content: tracePlan, TokensUsed: 10}, nil
}

func (p *provenanceProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: tracePlan, Done: true}
	close(ch)
	return ch, nil
}

// rowRecorder keeps every audit row the loop writes, as the loop wrote it.
// The rows are read here rather than back out of the database so that these
// tests ask what the loop recorded and nothing about which columns exist.
type rowRecorder struct {
	storage.StorageAdapter
	mu   sync.Mutex
	rows []storage.ToolEvent
}

func (r *rowRecorder) SaveToolEvent(ctx context.Context, e storage.ToolEvent) error {
	r.mu.Lock()
	r.rows = append(r.rows, e)
	r.mu.Unlock()
	return r.StorageAdapter.SaveToolEvent(ctx, e)
}

// ofKind returns the recorded rows of one kind. The act step leaves Kind empty
// and lets storage default it, so an empty kind is an execute row.
func (r *rowRecorder) ofKind(kind string) []storage.ToolEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []storage.ToolEvent
	for _, e := range r.rows {
		k := e.Kind
		if k == "" {
			k = storage.ToolEventExecute
		}
		if k == kind {
			out = append(out, e)
		}
	}
	return out
}

// provenanceFixture is traceLoopFixture with a provider that names its model,
// a recorder in front of the store, and the given objective saved.
func provenanceFixture(t *testing.T, p *provenanceProvider, obj objective.Objective) (*serviceImpl, *rowRecorder) {
	t.Helper()
	svc, _, _ := traceLoopFixture(t)

	providers := llm.NewRegistry(nil)
	providers.Register(p)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	rec := &rowRecorder{StorageAdapter: svc.store}
	svc.store = rec
	if err := rec.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return svc, rec
}

func provenanceAgent(provider string, bounds coreagent.AuthorityBounds) coreagent.Definition {
	return coreagent.Definition{
		ID:                "provenance-agent",
		Name:              "Provenance Agent",
		Domain:            "test",
		ReasoningStrategy: coreagent.ReasoningChainOfThought,
		Authority:         bounds,
		LLMHints:          capability.LLMHints{PreferredProvider: provider},
	}
}

// wantProvenance is what a row should say produced its decision. effective is
// the threshold the decide step actually applied.
type wantProvenance struct {
	provider, model, template string
	rung                      objective.AutonomyLevel
	effective                 float64
}

// assertProvenance checks the columns and the bounds in the payload against the
// agent definition the run was given.
func assertProvenance(t *testing.T, row storage.ToolEvent, def coreagent.Definition, want wantProvenance) {
	t.Helper()
	if row.AgentID != string(def.ID) {
		t.Errorf("agent id = %q, want %q", row.AgentID, def.ID)
	}
	if row.Provider != want.provider || row.Model != want.model {
		t.Errorf("provider/model = %q/%q, want %q/%q", row.Provider, row.Model, want.provider, want.model)
	}
	if row.TemplateID != want.template {
		t.Errorf("template id = %q, want %q", row.TemplateID, want.template)
	}
	if row.AutonomyRung != string(want.rung) {
		t.Errorf("autonomy rung = %q, want %q", row.AutonomyRung, want.rung)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v: %s", err, row.PayloadJSON)
	}
	if got := payload["reasoning_strategy"]; got != string(def.ReasoningStrategy) {
		t.Errorf("payload reasoning_strategy = %v, want %q", got, def.ReasoningStrategy)
	}
	if got := payload["max_autonomous"]; got != float64(def.Authority.MaxAutonomousActions) {
		t.Errorf("payload max_autonomous = %v, want %d", got, def.Authority.MaxAutonomousActions)
	}
	if got := payload["confidence_threshold"]; got != def.Authority.ConfidenceThreshold {
		t.Errorf("payload confidence_threshold = %v, want %v", got, def.Authority.ConfidenceThreshold)
	}
	if got := payload["effective_threshold"]; got != want.effective {
		t.Errorf("payload effective_threshold = %v, want %v", got, want.effective)
	}
	gated, _ := payload["requires_approval_for"].([]any)
	if len(gated) != len(def.Authority.RequiresApprovalFor) {
		t.Fatalf("payload requires_approval_for = %v, want %v", payload["requires_approval_for"], def.Authority.RequiresApprovalFor)
	}
	for i, id := range def.Authority.RequiresApprovalFor {
		if gated[i] != string(id) {
			t.Errorf("payload requires_approval_for[%d] = %v, want %q", i, gated[i], id)
		}
	}
}

// decisionCases are the two rows a decision leaves: the escalation when the
// loop declines to act, and the execute row when it acts.
var decisionCases = map[string]struct {
	bounds  coreagent.AuthorityBounds
	pending *corecheckpoint.Decision
	kind    string
}{
	"escalation": {
		// Zero autonomous actions: every non-empty plan escalates.
		bounds:  coreagent.AuthorityBounds{MaxAutonomousActions: 0},
		pending: &corecheckpoint.Decision{Choice: "reject", Approver: "ada"},
		kind:    storage.ToolEventEscalation,
	},
	"execute": {
		bounds: coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions},
		kind:   storage.ToolEventExecute,
	},
}

// An escalation is the loop declining to act. The row that records it has to
// say what drafted the plan it declined, under which bounds, at which rung and
// for an objective built from which template, or the question "why did this
// stop here" is answered by joining four tables that may since have changed.
func TestEscalationRowSaysWhatProducedTheDecision(t *testing.T) {
	obj := objective.Objective{ID: "obj-esc", Title: "keep the build green", Domain: "test", TemplateID: "tmpl-green-build"}
	svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
	def := provenanceAgent("anthropic", coreagent.AuthorityBounds{
		MaxAutonomousActions: 0,
		ConfidenceThreshold:  0.6,
		RequiresApprovalFor:  []capability.CapabilityID{"test.act.gated"},
	})
	reject := corecheckpoint.Decision{Choice: "reject", Approver: "ada"}

	runSync(svc, "loop-prov-0001", loop.Request{
		Objective: obj, Agent: def, MaxIter: 3, AutonomyRung: objective.AutonomyPropose,
	}, &reject)

	rows := rec.ofKind(storage.ToolEventEscalation)
	if len(rows) != 1 {
		t.Fatalf("escalation rows = %d, want 1", len(rows))
	}
	assertProvenance(t, rows[0], def, wantProvenance{
		provider: "anthropic", model: "model-a",
		template: "tmpl-green-build",
		rung:     objective.AutonomyPropose,
		// Nobody lowered it, so the bar applied is the bar declared.
		effective: 0.6,
	})
}

// The same for an action the loop took without asking, which is the row
// somebody reads when they want to know who let it.
func TestExecuteRowSaysWhatProducedTheDecision(t *testing.T) {
	obj := objective.Objective{ID: "obj-exec", Title: "keep the build green", Domain: "test", TemplateID: "tmpl-green-build"}
	svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
	def := provenanceAgent("anthropic", coreagent.AuthorityBounds{
		MaxAutonomousActions: coreagent.UnlimitedActions,
		ConfidenceThreshold:  0.6,
		RequiresApprovalFor:  []capability.CapabilityID{"test.act.gated"},
	})

	runSync(svc, "loop-prov-0002", loop.Request{
		Objective: obj, Agent: def, MaxIter: 1, AutonomyRung: objective.AutonomyAct,
	}, nil)

	if n := len(rec.ofKind(storage.ToolEventEscalation)); n != 0 {
		t.Fatalf("escalation rows = %d, want 0: the plan was within its bounds", n)
	}
	rows := rec.ofKind(storage.ToolEventExecute)
	if len(rows) != 1 {
		t.Fatalf("execute rows = %d, want 1", len(rows))
	}
	assertProvenance(t, rows[0], def, wantProvenance{
		provider: "anthropic", model: "model-a",
		template:  "tmpl-green-build",
		rung:      objective.AutonomyAct,
		effective: 0.6,
	})
}

// An operator who resolves a checkpoint with a revised confidence lowers the
// bar for that iteration. The action that then runs ran under the lowered
// bar, and its row has to say so beside the one the agent declares: recording
// only the declared threshold would show an action that could not have passed
// its own bounds.
func TestExecuteRowAfterAModifyRecordsTheThresholdThatApplied(t *testing.T) {
	obj := objective.Objective{ID: "obj-mod", Title: "keep the build green", Domain: "test"}
	svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
	// The plan is drafted at 0.95, under a bar of 0.99: it escalates on
	// confidence and on nothing else.
	def := provenanceAgent("anthropic", coreagent.AuthorityBounds{
		MaxAutonomousActions: coreagent.UnlimitedActions,
		ConfidenceThreshold:  0.99,
	})
	revised := 0.5
	modify := corecheckpoint.Decision{
		Choice: "modify", Approver: "ada",
		Modifications: &corecheckpoint.Modifications{RevisedConfidence: &revised},
	}

	runSync(svc, "loop-prov-0003", loop.Request{Objective: obj, Agent: def, MaxIter: 1}, &modify)

	escalations := rec.ofKind(storage.ToolEventEscalation)
	if len(escalations) != 1 {
		t.Fatalf("escalation rows = %d, want 1", len(escalations))
	}
	assertProvenance(t, escalations[0], def, wantProvenance{
		provider: "anthropic", model: "model-a", effective: 0.99,
	})

	executes := rec.ofKind(storage.ToolEventExecute)
	if len(executes) != 1 {
		t.Fatalf("execute rows = %d, want 1: the modified plan was within the lowered bar", len(executes))
	}
	assertProvenance(t, executes[0], def, wantProvenance{
		provider: "anthropic", model: "model-a", effective: revised,
	})
}

// A one-shot run was started by nobody's ladder, and an objective written by
// hand came from no template. Both rows say so by leaving the field empty,
// while still naming the model: an absent rung must not read as an absent
// record.
func TestOneShotRunRecordsNoRungAndNoTemplate(t *testing.T) {
	for name, tc := range decisionCases {
		t.Run(name, func(t *testing.T) {
			obj := objective.Objective{ID: "obj-oneshot", Title: "ship the release", Domain: "test"}
			svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
			def := provenanceAgent("anthropic", tc.bounds)

			runSync(svc, "loop-prov-0004", loop.Request{Objective: obj, Agent: def, MaxIter: 1}, tc.pending)

			rows := rec.ofKind(tc.kind)
			if len(rows) != 1 {
				t.Fatalf("%s rows = %d, want 1", tc.kind, len(rows))
			}
			assertProvenance(t, rows[0], def, wantProvenance{provider: "anthropic", model: "model-a"})
		})
	}
}

// The placeholder plan built when the model call fails was drafted by no
// model, and the rows a decision on it leaves behind must not name one. The
// agent's preferred provider is right there on the definition and is the wrong
// answer: it is who was asked, not who answered. Everything else about the
// decision is still known, and still recorded.
func TestFallbackPlanRowsNameNoModel(t *testing.T) {
	for name, tc := range decisionCases {
		t.Run(name, func(t *testing.T) {
			obj := objective.Objective{ID: "obj-fallback", Title: "keep the build green", Domain: "test", TemplateID: "tmpl-green-build"}
			svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a", fail: true}, obj)
			def := provenanceAgent("anthropic", tc.bounds)

			runSync(svc, "loop-prov-0005", loop.Request{
				Objective: obj, Agent: def, MaxIter: 1, AutonomyRung: objective.AutonomyActWithNotice,
			}, tc.pending)

			rows := rec.ofKind(tc.kind)
			if len(rows) != 1 {
				t.Fatalf("%s rows = %d, want 1", tc.kind, len(rows))
			}
			assertProvenance(t, rows[0], def, wantProvenance{
				template: "tmpl-green-build",
				rung:     objective.AutonomyActWithNotice,
			})
		})
	}
}
