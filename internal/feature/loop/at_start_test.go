package loop

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 41 step 7 part C: an at_start baseline is measured once, when the
// objective's first run starts, by the criterion's named verifier through the
// same dispatch the plan's actions use, before the first plan's actions run.
// The value is frozen into BaselineValue, persisted, and audited once; it is
// never measured or rewritten again. An inconclusive first measurement leaves
// no baseline for the life of the objective, so a relative target is
// inconclusive: no_baseline, never not_met (docs/roadmap.md, Phase 41
// decision 3 'Baselines' and step 7's acceptance).
//
// The measurement is the loop dispatching one action it did not plan, not a
// gate: nothing is refused on its account (AGENTS.md rule 8).

const (
	atStartVerifier = "test.verify.latency"
	atStartWork     = "test.act.ok"
	atStartEnv      = "test.env.ok"
)

// atStartPlan does the work and then runs the verifier, so every iteration's
// verify step has a measurement of its own to settle the criterion with.
const atStartPlan = `{"actions":[` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","reason":"do the work"},` +
	`{"capability":"test.verify.latency","env_id":"test.env.ok","reason":"measure"}` +
	`],"confidence":0.95,"reasoning":"r"}`

type verifierReply struct {
	result environment.ActionResult
	err    error
}

// atStartEnvFake serves the work capability and the verifier. It records every
// capability it is asked to act on, in order, and answers the verifier from a
// script: the nth call gets the nth reply, and the last reply repeats. The
// commit it is at is both its snapshot SHA and its observation version.
type atStartEnvFake struct {
	mu       sync.Mutex
	commit   string
	calls    []string
	verifier []verifierReply
	nVerify  int
	work     environment.ActionResult
}

func (e *atStartEnvFake) ID() environment.EnvironmentID { return atStartEnv }
func (e *atStartEnvFake) Domain() string                { return "test" }

func (e *atStartEnvFake) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return environment.Observation{EnvID: atStartEnv, Version: e.commit, Timestamp: time.Now().UTC()}, nil
}

func (e *atStartEnvFake) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, string(a.CapabilityID))
	if a.CapabilityID != atStartVerifier {
		return e.work, nil
	}
	reply := e.verifier[min(e.nVerify, len(e.verifier)-1)]
	e.nVerify++
	return reply.result, reply.err
}

func (e *atStartEnvFake) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (e *atStartEnvFake) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return environment.EnvironmentSnapshot{SHA: e.commit, EnvID: atStartEnv}, nil
}

func (e *atStartEnvFake) setCommit(sha string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.commit = sha
}

func (e *atStartEnvFake) actCalls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.calls)
}

func (e *atStartEnvFake) verifierCalls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nVerify
}

func latencyMeasured(value float64) verifierReply {
	return verifierReply{result: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
		{Name: "p95_latency_ms", Value: value, Unit: "ms", Source: atStartVerifier},
	}}}
}

// atStartCriterion is p95 latency not more than 10% above a baseline measured
// at start and not measured yet.
func atStartCriterion() objective.Criterion {
	target := 10.0
	return objective.Criterion{
		ID:          "latency",
		Description: "p95 latency is not more than 10% above where it started",
		Kind:        objective.KindMeasured,
		Verifier:    atStartVerifier,
		Metric:      "p95_latency_ms",
		Op:          objective.OpLessEqual,
		Target:      &target,
		Unit:        "ms",
		Weight:      1.0,
		Baseline:    &objective.Baseline{AtStart: true},
		Change:      objective.ChangePercent,
	}
}

// atStartFixture runs the whole loop against one environment serving the work
// and the verifier, with a planner that always plans plan. The objective is
// saved with the at_start criterion and no baseline value.
func atStartFixture(t *testing.T, env *atStartEnvFake, plan string) (*serviceImpl, storage.StorageAdapter, objective.Objective) {
	t.Helper()
	svc, _, store := newResumeFixture(t)
	svc.otel = observability.NewOTel(nil)
	svc.memSvc = featurememory.NewService(store, 5)

	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: plan})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  atStartEnv,
		Domain: "test",
		Serves: []capability.CapabilityID{atStartWork, atStartVerifier},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return env, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	obj := objective.Objective{
		ID: "obj-at-start", Title: "keep latency where it started", Domain: "test",
		SuccessCriteria: []objective.Criterion{atStartCriterion()},
	}
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return svc, store, obj
}

func runAtStart(t *testing.T, svc *serviceImpl, obj objective.Objective, loopID string, maxIter int) {
	t.Helper()
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	runSync(svc, loopID, loop.Request{Objective: obj, Agent: def, MaxIter: maxIter}, nil)
}

// storedLatency is the latency criterion as the objective store holds it.
func storedLatency(t *testing.T, store storage.StorageAdapter, id objective.ObjectiveID) objective.Criterion {
	t.Helper()
	stored, err := store.GetObjective(context.Background(), id)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	return criterionByID(t, stored.SuccessCriteria, "latency")
}

// baselineRecords are the audit rows of kind baseline for the objective,
// with their payloads decoded.
func baselineRecords(t *testing.T, store storage.StorageAdapter, id objective.ObjectiveID) []map[string]any {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(id)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []map[string]any
	for _, r := range rows {
		if r.Kind != storage.ToolEventBaseline {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("baseline row %s: payload is not JSON: %v (%q)", r.ID, err, r.PayloadJSON)
		}
		out = append(out, p)
	}
	return out
}

func frozenAt(c objective.Criterion) string {
	if c.BaselineValue == nil {
		return "nil"
	}
	b, _ := json.Marshal(*c.BaselineValue)
	return string(b)
}

// The acceptance case: the first run measures the baseline once with the named
// verifier before the plan's work, freezes 200 with the commit it was taken
// at, persists it and audits it in one row. The plan's own measurement, 215,
// is then 7.5% above it and met.
func TestAnAtStartBaselineIsMeasuredOnceBeforeTheFirstPlan(t *testing.T) {
	env := &atStartEnvFake{
		commit:   "c0ffee1",
		verifier: []verifierReply{latencyMeasured(200), latencyMeasured(215)},
		work:     environment.ActionResult{Success: true},
	}
	svc, store, obj := atStartFixture(t, env, atStartPlan)

	runAtStart(t, svc, obj, "loop-at-start-0001", 1)

	calls := env.actCalls()
	want := []string{atStartVerifier, atStartWork, atStartVerifier}
	if !slices.Equal(calls, want) {
		t.Errorf("act calls = %v, want %v: the at_start baseline is the verifier dispatched once, "+
			"through the same environment Act the plan's actions use, before the first plan's actions; "+
			"if the loop has no step before act that can dispatch it, that step is what part C adds", calls, want)
	}

	c := storedLatency(t, store, obj.ID)
	if c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Errorf("stored baseline_value = %s, want 200, the value measured at start, frozen and persisted", frozenAt(c))
	}

	records := baselineRecords(t, store, obj.ID)
	if len(records) != 1 {
		t.Fatalf("%d baseline audit rows, want exactly 1: %v", len(records), records)
	}
	r := records[0]
	for key, want := range map[string]any{
		"criterion": "latency",
		"verifier":  atStartVerifier,
		"value":     200.0,
		"unit":      "ms",
		"commit":    "c0ffee1",
	} {
		if r[key] != want {
			t.Errorf("baseline row %s = %#v, want %#v (row %v)", key, r[key], want, r)
		}
	}

	st := finalLoopState(t, store, "loop-at-start-0001")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionMet {
		t.Errorf("215 against a frozen 200 +10%%: state = %q (reason %q), want met", got.State, got.Reason)
	}
}

// Later iterations and later runs never measure again and never rewrite it.
// The verifier reads 300 and then 400 and the commit moves, and the baseline
// stays the 200 frozen at start, with still one audit row; each of those
// readings is settled against 200, so it is not met.
func TestAnAtStartBaselineIsNeverMeasuredOrRewrittenAgain(t *testing.T) {
	env := &atStartEnvFake{
		commit:   "c0ffee1",
		verifier: []verifierReply{latencyMeasured(200), latencyMeasured(300), latencyMeasured(400)},
		work:     environment.ActionResult{Success: true},
	}
	svc, store, obj := atStartFixture(t, env, atStartPlan)

	// Two iterations: neither 300 nor 400 is within 10% of 200, so the first
	// run does not converge after its first iteration.
	runAtStart(t, svc, obj, "loop-at-start-0002", 2)
	env.setCommit("beefcafe")
	runAtStart(t, svc, obj, "loop-at-start-0003", 1)

	// One baseline measurement, then one verifier action per planned
	// iteration: two in the first run, one in the second.
	if got := env.verifierCalls(); got != 4 {
		t.Errorf("verifier ran %d times over two runs of three iterations, want 4: "+
			"one at_start measurement and one per plan (calls %v)", got, env.actCalls())
	}

	c := storedLatency(t, store, obj.ID)
	if c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Errorf("stored baseline_value = %s after a second iteration and a second run, want the 200 frozen at start", frozenAt(c))
	}
	if records := baselineRecords(t, store, obj.ID); len(records) != 1 {
		t.Errorf("%d baseline audit rows after two runs, want exactly 1: %v", len(records), records)
	}

	st := finalLoopState(t, store, "loop-at-start-0003")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionNotMet {
		t.Errorf("400 against the frozen 200 +10%%: state = %q (reason %q), want not_met", got.State, got.Reason)
	}
}

// An at_start measurement that settled nothing leaves no baseline, for the
// life of the objective. The fact is audited once, with no value and the
// reason, and the criterion is inconclusive: no_baseline in both runs, never
// not_met. The plan's later trusted 190, on another commit in the second run,
// does not fill it.
func TestAnInconclusiveAtStartMeasurementLeavesNoBaseline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  verifierReply
		reason objective.InconclusiveReason
	}{
		{
			name:   "no measurement",
			first:  verifierReply{result: environment.ActionResult{Success: true}},
			reason: objective.ReasonNoData,
		},
		{
			name:   "verifier error",
			first:  verifierReply{err: errors.New("benchmark harness crashed")},
			reason: objective.ReasonVerifierError,
		},
		{
			name:   "unsupported",
			first:  verifierReply{err: versioncontrol.ErrUnsupported},
			reason: objective.ReasonUnsupported,
		},
		{
			name: "third-party value",
			first: verifierReply{result: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
				{Name: "p95_latency_ms", Value: 200, Unit: "ms", Source: atStartVerifier, Trust: environment.TrustThirdParty},
			}}},
			reason: objective.ReasonNoData,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := &atStartEnvFake{
				commit:   "c0ffee1",
				verifier: []verifierReply{tc.first, latencyMeasured(190)},
				work:     environment.ActionResult{Success: true},
			}
			svc, store, obj := atStartFixture(t, env, atStartPlan)

			runAtStart(t, svc, obj, "loop-at-start-0004", 1)
			if calls := env.actCalls(); len(calls) == 0 || calls[0] != atStartVerifier {
				t.Errorf("act calls = %v: the first run's first action is the at_start measurement", calls)
			}
			if c := storedLatency(t, store, obj.ID); c.BaselineValue != nil {
				t.Errorf("stored baseline_value = %s after an inconclusive at_start measurement, want nil", frozenAt(c))
			}
			st := finalLoopState(t, store, "loop-at-start-0004")
			if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoBaseline {
				t.Errorf("first run: state = %q, reason = %q; want inconclusive: no_baseline", got.State, got.Reason)
			}

			env.setCommit("beefcafe")
			runAtStart(t, svc, obj, "loop-at-start-0005", 1)

			if c := storedLatency(t, store, obj.ID); c.BaselineValue != nil {
				t.Errorf("stored baseline_value = %s after a later run on another commit, want nil: "+
					"no later measurement or other commit's value is a baseline", frozenAt(c))
			}
			st = finalLoopState(t, store, "loop-at-start-0005")
			if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoBaseline {
				t.Errorf("second run: state = %q, reason = %q; want inconclusive: no_baseline", got.State, got.Reason)
			}
			if got := env.verifierCalls(); got != 3 {
				t.Errorf("verifier ran %d times over two one-iteration runs, want 3: "+
					"one at_start measurement and one per plan, never a second attempt at the baseline (calls %v)", got, env.actCalls())
			}

			records := baselineRecords(t, store, obj.ID)
			if len(records) != 1 {
				t.Fatalf("%d baseline audit rows, want exactly 1 saying the measurement was inconclusive: %v", len(records), records)
			}
			r := records[0]
			if v, ok := r["value"]; ok && v != nil {
				t.Errorf("baseline row value = %#v, want none for an inconclusive measurement", v)
			}
			for key, want := range map[string]any{
				"criterion": "latency",
				"verifier":  atStartVerifier,
				"unit":      "ms",
				"commit":    "c0ffee1",
				"reason":    string(tc.reason),
			} {
				if r[key] != want {
					t.Errorf("baseline row %s = %#v, want %#v (row %v)", key, r[key], want, r)
				}
			}
		})
	}
}

// Only the criterion's named verifier sets the baseline. The work capability
// reports p95_latency_ms too, at 50, and the at_start measurement reports
// nothing, so there is no baseline: the other capability's figure is not one.
func TestOnlyTheNamedVerifierSetsTheBaseline(t *testing.T) {
	env := &atStartEnvFake{
		commit:   "c0ffee1",
		verifier: []verifierReply{{result: environment.ActionResult{Success: true}}, latencyMeasured(190)},
		work: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
			{Name: "p95_latency_ms", Value: 50, Unit: "ms", Source: atStartWork},
		}},
	}
	svc, store, obj := atStartFixture(t, env, atStartPlan)

	runAtStart(t, svc, obj, "loop-at-start-0006", 1)

	if c := storedLatency(t, store, obj.ID); c.BaselineValue != nil {
		t.Errorf("stored baseline_value = %s, want nil: only %s measures the baseline, not %s", frozenAt(c), atStartVerifier, atStartWork)
	}
	records := baselineRecords(t, store, obj.ID)
	if len(records) != 1 {
		t.Fatalf("%d baseline audit rows, want exactly 1: %v", len(records), records)
	}
	if v, ok := records[0]["value"]; ok && v != nil {
		t.Errorf("baseline row value = %#v, want none: the work capability's 50 is not the verifier's measurement", v)
	}
	st := finalLoopState(t, store, "loop-at-start-0006")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoBaseline {
		t.Errorf("state = %q, reason = %q; want inconclusive: no_baseline", got.State, got.Reason)
	}
}
