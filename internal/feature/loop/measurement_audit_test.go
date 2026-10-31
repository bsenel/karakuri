package loop

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 step 9 slice 9b: every measurement an action returns is audited
// once, in a row of kind measurement, with the capability that took it, the
// action's parameters, the value, unit, source and trust, and the commit it
// was taken at (docs/roadmap.md, Phase 41 step 9, and decision 6
// 'Measurements are audited'). A measurement that settles nothing is audited
// all the same: "a measurement from any other capability ... is audited but
// settles nothing" (decision 6 'Only the named verifier, only this commit').

// measuredPlan does the work and then runs the verifier with parameters, so
// the audit row has parameters to carry.
const measuredPlan = `{"actions":[` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","reason":"do the work"},` +
	`{"capability":"test.verify.latency","env_id":"test.env.ok","params":{"route":"/api/v1/objectives","runs":3},"reason":"measure"}` +
	`],"confidence":0.95,"reasoning":"r"}`

// committingEnv is atStartEnvFake whose work capability moves the checkout to
// afterWork, as a pass that commits does. The verifier runs after the work, so
// the commit the verify step judges, and the one every measurement it reads
// was taken at, is afterWork, not the commit the run started on.
type committingEnv struct {
	*atStartEnvFake
	afterWork string
}

func (e *committingEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	res, err := e.atStartEnvFake.Act(ctx, a)
	if a.CapabilityID == atStartWork {
		e.setCommit(e.afterWork)
	}
	return res, err
}

// fixedLatencyCriterion is p95 latency at most 250 ms. It has no baseline, so
// no at_start measurement runs and every measurement in the run is the plan's.
func fixedLatencyCriterion() objective.Criterion {
	target := 250.0
	return objective.Criterion{
		ID:          "latency",
		Description: "p95 latency is at most 250 ms",
		Kind:        objective.KindMeasured,
		Verifier:    atStartVerifier,
		Metric:      "p95_latency_ms",
		Op:          objective.OpLessEqual,
		Target:      &target,
		Unit:        "ms",
		Weight:      1.0,
	}
}

// measurementAuditFixture runs one iteration of the whole loop against env,
// with the fixed latency criterion and measuredPlan.
func measurementAuditFixture(t *testing.T, env *committingEnv, loopID string) (storage.StorageAdapter, objective.Objective) {
	t.Helper()
	svc, store, obj := atStartFixture(t, env.atStartEnvFake, measuredPlan)
	// atStartFixture registers the inner fake; route the same capabilities to
	// the wrapper so the work moves the commit.
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

	obj.SuccessCriteria = []objective.Criterion{fixedLatencyCriterion()}
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	runAtStart(t, svc, obj, loopID, 1)
	return store, obj
}

// measurementRow is one measurement audit row: the row itself and its payload
// decoded field by field.
type measurementRow struct {
	row     storage.ToolEvent
	payload map[string]any
}

func measurementRows(t *testing.T, store storage.StorageAdapter, id objective.ObjectiveID) []measurementRow {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(id)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []measurementRow
	for _, r := range rows {
		if r.Kind != storage.ToolEventMeasurement {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("measurement row %s: payload is not JSON: %v (%q)", r.ID, err, r.PayloadJSON)
		}
		out = append(out, measurementRow{row: r, payload: p})
	}
	return out
}

// rowFor is the one measurement row for metric, failing when there is not
// exactly one: each measurement is audited once.
func rowFor(t *testing.T, rows []measurementRow, metric string) measurementRow {
	t.Helper()
	var found []measurementRow
	for _, r := range rows {
		if r.payload["metric"] == metric {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d measurement rows for metric %q, want exactly 1 (rows %v)", len(found), metric, rows)
	}
	return found[0]
}

// assertMeasurementRow checks every audited field by name. Trust is required
// to be present even when it is the operator's, which is the empty string, so
// a row that dropped the key does not pass for a trusted one.
func assertMeasurementRow(t *testing.T, r measurementRow, want map[string]any) {
	t.Helper()
	for key, w := range want {
		got, ok := r.payload[key]
		if !ok {
			t.Errorf("measurement row has no %q field, want %#v (payload %v)", key, w, r.payload)
			continue
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("measurement row %s = %#v, want %#v (payload %v)", key, got, w, r.payload)
		}
	}
	if verifier, _ := want["verifier"].(string); r.row.Capability != verifier {
		t.Errorf("measurement row Capability = %q, want %q, the capability that took it", r.row.Capability, verifier)
	}
}

// The named verifier returns two measurements; each is audited once with the
// verifier, the action's parameters, its value, unit, source and trust, and the
// commit the verify step judged, the one the work moved the checkout to. The
// trusted 215 ms settles the criterion met.
//
// docs/roadmap.md, Phase 41 step 9: "The audit row carries the verifier,
// parameters, value, unit, source, trust and commit."
func TestEachMeasurementIsAuditedOnceWithItsVerifierAndCommit(t *testing.T) {
	env := &committingEnv{
		atStartEnvFake: &atStartEnvFake{
			commit: "c0ffee1",
			verifier: []verifierReply{{result: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
				{Name: "p95_latency_ms", Value: 215, Unit: "ms", Source: "bench/latency.k6"},
				{Name: "requests_total", Value: 1000, Unit: "1", Source: "bench/latency.k6"},
			}}}},
			work: environment.ActionResult{Success: true},
		},
		afterWork: "beefcafe",
	}
	store, obj := measurementAuditFixture(t, env, "loop-measure-audit-0001")

	rows := measurementRows(t, store, obj.ID)
	if len(rows) != 2 {
		t.Fatalf("%d measurement audit rows, want 2, one per measurement the verifier returned: %v", len(rows), rows)
	}
	params := map[string]any{"route": "/api/v1/objectives", "runs": 3.0}
	assertMeasurementRow(t, rowFor(t, rows, "p95_latency_ms"), map[string]any{
		"verifier": atStartVerifier,
		"params":   params,
		"metric":   "p95_latency_ms",
		"value":    215.0,
		"unit":     "ms",
		"source":   "bench/latency.k6",
		"trust":    string(environment.TrustOperator),
		"commit":   "beefcafe",
	})
	assertMeasurementRow(t, rowFor(t, rows, "requests_total"), map[string]any{
		"verifier": atStartVerifier,
		"params":   params,
		"metric":   "requests_total",
		"value":    1000.0,
		"unit":     "1",
		"source":   "bench/latency.k6",
		"trust":    string(environment.TrustOperator),
		"commit":   "beefcafe",
	})

	st := finalLoopState(t, store, "loop-measure-audit-0001")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionMet {
		t.Errorf("215 ms against at most 250 ms: state = %q (reason %q), want met", got.State, got.Reason)
	}
}

// A third-party figure is audited with trust third_party, and it still
// settles no required criterion (step 4, AGENTS.md rule 9).
//
// docs/roadmap.md, Phase 41 step 9.
func TestAThirdPartyMeasurementIsAuditedWithItsTrust(t *testing.T) {
	env := &committingEnv{
		atStartEnvFake: &atStartEnvFake{
			commit: "c0ffee1",
			verifier: []verifierReply{{result: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
				{Name: "p95_latency_ms", Value: 215, Unit: "ms", Source: "https://status.example.com", Trust: environment.TrustThirdParty},
			}}}},
			work: environment.ActionResult{Success: true},
		},
		afterWork: "beefcafe",
	}
	store, obj := measurementAuditFixture(t, env, "loop-measure-audit-0002")

	rows := measurementRows(t, store, obj.ID)
	if len(rows) != 1 {
		t.Fatalf("%d measurement audit rows, want 1: %v", len(rows), rows)
	}
	assertMeasurementRow(t, rows[0], map[string]any{
		"verifier": atStartVerifier,
		"params":   map[string]any{"route": "/api/v1/objectives", "runs": 3.0},
		"metric":   "p95_latency_ms",
		"value":    215.0,
		"unit":     "ms",
		"source":   "https://status.example.com",
		"trust":    string(environment.TrustThirdParty),
		"commit":   "beefcafe",
	})

	st := finalLoopState(t, store, "loop-measure-audit-0002")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive {
		t.Errorf("a third-party 215 ms on a required criterion: state = %q (reason %q), want inconclusive: "+
			"it is audited, and it settles nothing", got.State, got.Reason)
	}
}

// A measurement from a capability other than the named verifier is audited
// with that capability, and it settles nothing: the verifier measured
// nothing, so the criterion is inconclusive however good the work's 50 ms is.
//
// docs/roadmap.md, Phase 41 step 9, and decision 6 'Only the named verifier,
// only this commit'.
func TestAnotherCapabilitysMeasurementIsAuditedAndSettlesNothing(t *testing.T) {
	env := &committingEnv{
		atStartEnvFake: &atStartEnvFake{
			commit:   "c0ffee1",
			verifier: []verifierReply{{result: environment.ActionResult{Success: true}}},
			work: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
				{Name: "p95_latency_ms", Value: 50, Unit: "ms", Source: "work/self-report"},
			}},
		},
		afterWork: "beefcafe",
	}
	store, obj := measurementAuditFixture(t, env, "loop-measure-audit-0003")

	rows := measurementRows(t, store, obj.ID)
	if len(rows) != 1 {
		t.Fatalf("%d measurement audit rows, want 1, the work capability's: %v", len(rows), rows)
	}
	r := rows[0]
	// The commit the work's own figure was taken at is the one its action
	// started on; only its presence is pinned here, the value is the code's
	// to document.
	if _, ok := r.payload["commit"]; !ok {
		t.Errorf("measurement row has no commit field (payload %v)", r.payload)
	}
	assertMeasurementRow(t, r, map[string]any{
		"verifier": atStartWork,
		"metric":   "p95_latency_ms",
		"value":    50.0,
		"unit":     "ms",
		"source":   "work/self-report",
		"trust":    string(environment.TrustOperator),
	})

	st := finalLoopState(t, store, "loop-measure-audit-0003")
	if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive {
		t.Errorf("only the work capability measured 50 ms: state = %q (reason %q), want inconclusive: "+
			"another capability's figure is audited and settles nothing", got.State, got.Reason)
	}
}

// A verifier result with no measurements writes no measurement row, and
// neither does work that returned none.
//
// docs/roadmap.md, Phase 41 step 9.
func TestAResultWithNoMeasurementsWritesNoMeasurementRow(t *testing.T) {
	env := &committingEnv{
		atStartEnvFake: &atStartEnvFake{
			commit:   "c0ffee1",
			verifier: []verifierReply{{result: environment.ActionResult{Success: true}}},
			work:     environment.ActionResult{Success: true},
		},
		afterWork: "beefcafe",
	}
	store, obj := measurementAuditFixture(t, env, "loop-measure-audit-0004")

	if rows := measurementRows(t, store, obj.ID); len(rows) != 0 {
		t.Errorf("%d measurement audit rows for results with no measurements, want 0: %v", len(rows), rows)
	}
}
