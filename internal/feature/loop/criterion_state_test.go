package loop

import (
	"context"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 Step 1: each criterion has a state, completion is "all required
// criteria met", and how a run ended is recorded apart from what its verifiers
// settled.

func optional() *bool { no := false; return &no }

// eventLog collects what a hub publishes for one objective, drained on its own
// goroutine so a busy run cannot overflow the subscription and drop the event
// a test is looking for.
type eventLog struct {
	mu     sync.Mutex
	events []event.Event
	done   chan struct{}
	unsub  func()
}

func collectEvents(hub *event.Hub, objID objective.ObjectiveID) *eventLog {
	ch, unsub := hub.Subscribe(context.Background(), "obj:"+string(objID))
	l := &eventLog{done: make(chan struct{}), unsub: unsub}
	go func() {
		defer close(l.done)
		for e := range ch {
			l.mu.Lock()
			l.events = append(l.events, e)
			l.mu.Unlock()
		}
	}()
	return l
}

// stop ends the subscription and returns everything it saw.
func (l *eventLog) stop() []event.Event {
	l.unsub()
	<-l.done
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.events
}

// verifyCompleted returns the payloads of the verify step's completion events.
func verifyCompleted(events []event.Event) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e.Type != event.TypeLoopStepCompleted {
			continue
		}
		p := e.Payload
		if p["step"] != string(loop.StepVerify) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func criterionByID(t *testing.T, cs []objective.Criterion, id string) objective.Criterion {
	t.Helper()
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no criterion %q in %+v", id, cs)
	return objective.Criterion{}
}

// The acceptance case. One required criterion met, one required criterion
// whose verifier could not run, and one optional criterion not met: the
// objective does not complete, and the step says why — the required one is
// inconclusive with verifier_error, not merely "not met".
func TestStepVerifyDoesNotCompleteOnAnInconclusiveRequiredCriterion(t *testing.T) {
	hub := event.NewHub()
	judge := &scriptedAgent{scripted: []coreagent.Output{{Content: "PASS"}, {Content: "PASS"}, {Content: "PASS"}}}
	sc := &stepContext{
		svc:   &serviceImpl{hub: hub},
		agent: judge,
		obj: objective.Objective{
			ID: "obj-states",
			SuccessCriteria: []objective.Criterion{
				{ID: "built", Description: "the build passes", Verifier: "test.verify.build"},
				{ID: "tested", Description: "the tests pass", Verifier: "test.verify.tests"},
				{ID: "tidy", Description: "lint is clean", Verifier: "test.verify.lint", Required: optional()},
			},
		},
	}
	log := collectEvents(hub, sc.obj.ID)

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "test.verify.build", Result: environment.ActionResult{Success: true}},
		// Never ran: the adapter refused the call.
		{CapabilityID: "test.verify.tests", Errored: true,
			Result: environment.ActionResult{Success: false, Error: "adapter refused the call"}},
		// Ran, and reported failure.
		{CapabilityID: "test.verify.lint", Result: environment.ActionResult{Success: false, Error: "3 findings"}},
	})
	events := log.stop()

	if allMet {
		t.Error("completed with a required criterion inconclusive")
	}

	want := map[string]struct {
		state  objective.CriterionState
		reason objective.InconclusiveReason
	}{
		"built":  {objective.CriterionMet, ""},
		"tested": {objective.CriterionInconclusive, objective.ReasonVerifierError},
		"tidy":   {objective.CriterionNotMet, ""},
	}
	for id, w := range want {
		c := criterionByID(t, sc.obj.SuccessCriteria, id)
		if c.State != w.state || c.Reason != w.reason {
			t.Errorf("%s: state=%q reason=%q, want %q/%q", id, c.State, c.Reason, w.state, w.reason)
		}
	}

	// The record of the step carries the states, so the reason is not lost
	// with the in-memory objective.
	payloads := verifyCompleted(events)
	if len(payloads) != 1 {
		t.Fatalf("verify completed %d times, want once", len(payloads))
	}
	recorded, ok := payloads[0]["criteria"].([]objective.Criterion)
	if !ok {
		t.Fatalf("verify payload carries no criteria states: %#v", payloads[0])
	}
	tested := criterionByID(t, recorded, "tested")
	if tested.State != objective.CriterionInconclusive || tested.Reason != objective.ReasonVerifierError {
		t.Errorf("recorded tested: state=%q reason=%q, want inconclusive/verifier_error", tested.State, tested.Reason)
	}
}

// A verifier that could not run is never handed to the judge: the error says
// nothing about the work, and a model reading it would be judging the adapter.
func TestAVerifierErrorIsNotJudged(t *testing.T) {
	judge := &scriptedAgent{scripted: []coreagent.Output{{Content: "PASS"}}}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: judge,
		obj: objective.Objective{
			ID: "obj-unjudged",
			SuccessCriteria: []objective.Criterion{
				{ID: "tested", Description: "the tests pass", Verifier: "test.verify.tests"},
			},
		},
	}

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "test.verify.tests", Errored: true,
			Result: environment.ActionResult{Success: false, Error: "connection refused"}},
	})

	if allMet {
		t.Error("a criterion whose verifier could not run completed the objective")
	}
	if judge.calls != 0 {
		t.Errorf("the judge was asked %d times about a verifier that never ran: %q", judge.calls, judge.tasksSeen)
	}
	c := sc.obj.SuccessCriteria[0]
	if c.State != objective.CriterionInconclusive || c.Reason != objective.ReasonVerifierError {
		t.Errorf("state=%q reason=%q, want inconclusive/verifier_error", c.State, c.Reason)
	}
}

// An optional criterion can neither block nor grant completion. Every
// required one met completes the objective, however the optional one went,
// and the optional one still counts towards the score.
func TestAnOptionalCriterionDoesNotBlockCompletion(t *testing.T) {
	sc := &stepContext{
		svc: &serviceImpl{hub: event.NewHub()},
		obj: objective.Objective{
			ID: "obj-optional",
			SuccessCriteria: []objective.Criterion{
				{ID: "tested", Verifier: "test.verify.tests", Weight: 1},
				{ID: "tidy", Verifier: "test.verify.lint", Weight: 1, Required: optional()},
			},
		},
	}

	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "test.verify.tests", Result: environment.ActionResult{Success: true}},
		{CapabilityID: "test.verify.lint", Result: environment.ActionResult{Success: false}},
	})

	if !allMet {
		t.Error("an optional criterion not met blocked completion")
	}
	if score != 0.5 {
		t.Errorf("score = %v, want 0.5 — the optional criterion still counts as progress", score)
	}
}

// Zero required criteria never completes. An objective whose criteria are all
// optional fails like an empty one, rather than completing vacuously because
// every one of them happened to be met.
func TestStepVerifyWithOnlyOptionalCriteriaFailsHonestly(t *testing.T) {
	hub := event.NewHub()
	sc := &stepContext{
		svc: &serviceImpl{hub: hub},
		obj: objective.Objective{
			ID: "obj-all-optional",
			SuccessCriteria: []objective.Criterion{
				{ID: "tidy", Verifier: "test.verify.lint", Required: optional()},
				{ID: "fast", Verifier: "test.verify.bench", Required: optional()},
			},
		},
	}
	log := collectEvents(hub, sc.obj.ID)

	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "test.verify.lint", Result: environment.ActionResult{Success: true}},
		{CapabilityID: "test.verify.bench", Result: environment.ActionResult{Success: true}},
	})
	events := log.stop()

	if allMet {
		t.Error("an objective with no required criterion completed")
	}
	if score != 0.0 {
		t.Errorf("score = %v, want 0.0, as for an objective with no criteria", score)
	}
	payloads := verifyCompleted(events)
	if len(payloads) != 1 || payloads[0]["reason"] != "no_success_criteria_defined" {
		t.Errorf("verify payloads = %v, want one with reason no_success_criteria_defined", payloads)
	}
}

// Existing objectives keep their scores: criteria that say nothing about
// required or state score exactly as they did before Phase 41.
func TestCriteriaWrittenBeforeStatesKeepTheirScores(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lintOK    bool
		wantScore float64
		wantMet   bool
	}{
		{"both met", true, 1.0, true},
		{"the lighter one not met", false, 0.75, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := &stepContext{
				svc: &serviceImpl{hub: event.NewHub()},
				obj: objective.Objective{
					ID: "obj-legacy",
					SuccessCriteria: []objective.Criterion{
						{ID: "tested", Verifier: "test.verify.tests", Weight: 3},
						{ID: "tidy", Verifier: "test.verify.lint", Weight: 1},
					},
				},
			}
			score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
				{CapabilityID: "test.verify.tests", Result: environment.ActionResult{Success: true}},
				{CapabilityID: "test.verify.lint", Result: environment.ActionResult{Success: tc.lintOK}},
			})
			if score != tc.wantScore || allMet != tc.wantMet {
				t.Errorf("(score, allMet) = (%v, %v), want (%v, %v)", score, allMet, tc.wantScore, tc.wantMet)
			}
		})
	}
}

// The act step is where "could not run" and "ran and failed" part ways: an
// environment that returned an error, or no environment at all, is Errored;
// one that ran and reported failure is not.
func TestStepActMarksAnActionThatCouldNotRun(t *testing.T) {
	sc := observeFixture(t,
		&traceEnv{id: "test.env.broken", failAct: true},
		&settleEnv{id: "test.env.red", result: environment.ActionResult{Success: false, Error: "2 tests failed"}},
	)

	outcomes := stepAct(context.Background(), sc, plan{
		Confidence: 0.9,
		Actions: []plannedAction{
			{CapabilityID: "test.act.broken", EnvID: "test.env.broken"},
			{CapabilityID: "test.act.red", EnvID: "test.env.red"},
			{CapabilityID: "test.act.nowhere", EnvID: "test.env.missing"},
		},
	})

	want := map[string]bool{
		"test.act.broken":  true,
		"test.act.red":     false,
		"test.act.nowhere": true,
	}
	if len(outcomes) != len(want) {
		t.Fatalf("outcomes = %d, want %d", len(outcomes), len(want))
	}
	for _, o := range outcomes {
		w, ok := want[o.CapabilityID]
		if !ok {
			t.Errorf("unexpected outcome for %s", o.CapabilityID)
			continue
		}
		if o.Errored != w {
			t.Errorf("%s: Errored = %v, want %v", o.CapabilityID, o.Errored, w)
		}
	}
}

// settleEnv answers every action with the result it was given and no Go error:
// a verifier that ran and said what it found.
type settleEnv struct {
	id     environment.EnvironmentID
	result environment.ActionResult
}

func (e *settleEnv) ID() environment.EnvironmentID { return e.id }
func (e *settleEnv) Domain() string                { return "test" }

func (e *settleEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: e.id, Version: string(e.id) + "-v1", Timestamp: time.Now().UTC()}, nil
}

func (e *settleEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return e.result, nil
}

func (e *settleEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (e *settleEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}

// settleLoopFixture is a service that runs the whole loop against one
// environment, test.env.ok serving test.act.ok, which every plan calls. The
// objective is saved with the given criteria, so the run is verified by them.
func settleLoopFixture(t *testing.T, env environment.Environment, criteria []objective.Criterion) (*serviceImpl, storage.StorageAdapter, objective.Objective) {
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
			return env, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	obj := objective.Objective{
		ID: "obj-settle", Title: "settle the criteria", Domain: "test",
		SuccessCriteria: criteria,
	}
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return svc, store, obj
}

func finalLoopState(t *testing.T, store storage.StorageAdapter, loopID string) loop.State {
	t.Helper()
	st, err := store.GetLoopState(context.Background(), loopID)
	if err != nil {
		t.Fatalf("get loop state: %v", err)
	}
	if !st.Completed {
		t.Fatalf("loop %s did not finish", loopID)
	}
	return st
}

// How a run ended is recorded apart from what its verifiers settled. A run
// that spends its iteration budget says so, and each criterion keeps the
// state its verifier gave it — the budget running out turns nothing into
// not_met, and nothing into met.
func TestARunRecordsItsOutcomeApartFromTheCriterionStates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		env         environment.Environment
		wantOutcome loop.RunOutcome
		wantStatus  objective.ObjectiveStatus
		wantState   objective.CriterionState
		wantReason  objective.InconclusiveReason
	}{
		{
			name:        "verifier passes",
			env:         &settleEnv{id: "test.env.ok", result: environment.ActionResult{Success: true}},
			wantOutcome: loop.RunConverged,
			wantStatus:  objective.StatusCompleted,
			wantState:   objective.CriterionMet,
		},
		{
			name:        "verifier ran and failed every iteration",
			env:         &settleEnv{id: "test.env.ok", result: environment.ActionResult{Success: false, Error: "2 tests failed"}},
			wantOutcome: loop.RunBudgetSpent,
			wantStatus:  objective.StatusFailed,
			wantState:   objective.CriterionNotMet,
		},
		{
			name:        "verifier never could run",
			env:         &traceEnv{id: "test.env.ok", failAct: true},
			wantOutcome: loop.RunBudgetSpent,
			wantStatus:  objective.StatusFailed,
			wantState:   objective.CriterionInconclusive,
			wantReason:  objective.ReasonVerifierError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, obj := settleLoopFixture(t, tc.env, []objective.Criterion{
				{ID: "tested", Description: "the tests pass", Verifier: "test.act.ok"},
			})
			def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})

			runSync(svc, "loop-settle-0001", loop.Request{Objective: obj, Agent: def, MaxIter: 2}, nil)

			st := finalLoopState(t, store, "loop-settle-0001")
			if st.Outcome != tc.wantOutcome {
				t.Errorf("outcome = %q, want %q", st.Outcome, tc.wantOutcome)
			}
			if st.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", st.Status, tc.wantStatus)
			}
			if len(st.Criteria) != 1 {
				t.Fatalf("loop state carries %d criteria, want the 1 it was verified by", len(st.Criteria))
			}
			c := st.Criteria[0]
			if c.State != tc.wantState || c.Reason != tc.wantReason {
				t.Errorf("criterion state=%q reason=%q, want %q/%q", c.State, c.Reason, tc.wantState, tc.wantReason)
			}
		})
	}
}

// A run that could not get going at all ended errored, which is a different
// fact from one that ran out of iterations.
func TestARunThatCouldNotStartRecordsErrored(t *testing.T) {
	svc, store, _ := settleLoopFixture(t, &settleEnv{id: "test.env.ok"}, nil)
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})

	// Never saved, so the run fails fetching it.
	missing := objective.Objective{ID: "obj-missing", Domain: "test"}
	runSync(svc, "loop-settle-0002", loop.Request{Objective: missing, Agent: def, MaxIter: 2}, nil)

	st := finalLoopState(t, store, "loop-settle-0002")
	if st.Outcome != loop.RunErrored {
		t.Errorf("outcome = %q, want %q", st.Outcome, loop.RunErrored)
	}
}

// The acceptance case for zero required criteria, through a whole run: every
// criterion optional and met, and the objective still ends failed, with the
// verify step saying no criteria were defined.
func TestARunWithOnlyOptionalCriteriaEndsFailed(t *testing.T) {
	svc, store, obj := settleLoopFixture(t,
		&settleEnv{id: "test.env.ok", result: environment.ActionResult{Success: true}},
		[]objective.Criterion{
			{ID: "tidy", Description: "lint is clean", Verifier: "test.act.ok", Required: optional()},
		})
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	log := collectEvents(svc.hub, obj.ID)

	runSync(svc, "loop-settle-0003", loop.Request{Objective: obj, Agent: def, MaxIter: 1}, nil)
	events := log.stop()

	st := finalLoopState(t, store, "loop-settle-0003")
	if st.Status != objective.StatusFailed {
		t.Errorf("loop status = %q, want %q", st.Status, objective.StatusFailed)
	}
	stored, err := store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	if stored.Status != objective.StatusFailed {
		t.Errorf("objective status = %q, want %q", stored.Status, objective.StatusFailed)
	}
	reasons := 0
	for _, p := range verifyCompleted(events) {
		if p["reason"] == "no_success_criteria_defined" {
			reasons++
		}
	}
	if reasons == 0 {
		t.Error("the verify step never said no required criteria were defined")
	}
}
