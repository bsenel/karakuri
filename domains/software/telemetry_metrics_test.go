package software

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
)

// Phase 41 step 13: cost_per_pass and escalation_approval_rate, measured from
// the deployment's own telemetry (docs/roadmap.md, Phase 41 section 7 item 8,
// step 13 and the risk "Two verifiers may not exist"; ADR 029).
//
// The sources, cited as the step 13 acceptance asks:
//   - a pass is a reconcile outcome carrying a LoopID, counted into
//     Snapshot.Work.Reconciles at internal/platform/telemetry/reader.go:113;
//   - spend is Snapshot.Spend{Cost, Priced}, read by readSpend at
//     internal/platform/telemetry/reader.go:226-259, where Priced stays false
//     when the cost report could not be read (:232-235) or when units were
//     counted at no cost (:256-258);
//   - approvals and rejections are counted from audit tool events at
//     internal/platform/telemetry/reader.go:174-175, and
//     EscalationStats.ApprovalRate (internal/core/telemetry/telemetry.go:83)
//     returns -1 when nothing was resolved.
//
// The environment that reads them is telemetryEnv (selfimprove.go), served by
// platformTelemetryFactory as software.env.platform_telemetry.

// fixedReader returns one snapshot, or an error, and records the query it was
// asked so a test can see the params reach the reader.
type fixedReader struct {
	snap  coretelemetry.Snapshot
	err   error
	calls int
	last  coretelemetry.Query
}

func (r *fixedReader) Snapshot(_ context.Context, q coretelemetry.Query) (coretelemetry.Snapshot, error) {
	r.calls++
	r.last = q
	return r.snap, r.err
}

// telemetryMetric is one of the two step 13 verifiers and what it measures.
type telemetryMetric struct {
	capID     string
	metric    string
	unit      string
	direction capability.Direction
}

var telemetryMetrics = []telemetryMetric{
	{CapCostPerPass, "cost_per_pass", "usd", capability.DirectionLowerIsBetter},
	{CapEscalationApprovalRate, "escalation_approval_rate", "ratio", capability.DirectionHigherIsBetter},
}

// pricedSnapshot is a window with four passes, 3.00 USD of priced spend, and
// three approvals against one rejection.
func pricedSnapshot() coretelemetry.Snapshot {
	return coretelemetry.Snapshot{
		Work:       coretelemetry.WorkStats{Senses: 10, Reconciles: 4},
		Escalation: coretelemetry.EscalationStats{Escalations: 5, Approvals: 3, Rejections: 1, Pending: 1},
		Spend:      coretelemetry.SpendStats{Cost: 3.0, Priced: true},
	}
}

func actTelemetry(t *testing.T, env *telemetryEnv, capID string, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: capability.CapabilityID(capID), Params: params})
	if err != nil {
		t.Fatalf("Act(%s) returned an error; a verifier reports what it could not measure in its result: %v", capID, err)
	}
	return res
}

// assertTelemetryMeasured requires exactly one first-party measurement of the
// metric, in its unit, with the value, and the state delta the step 12
// runtime verifiers carry.
func assertTelemetryMeasured(t *testing.T, res environment.ActionResult, m telemetryMetric, want float64) {
	t.Helper()
	if !res.Success {
		t.Fatalf("%s did not succeed: %s", m.capID, res.Error)
	}
	if res.Trust != environment.TrustOperator {
		t.Errorf("%s result trust = %q, want operator: the number is the deployment's own", m.capID, res.Trust)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("%s measured %+v, want exactly one measurement", m.capID, res.Measurements)
	}
	got := res.Measurements[0]
	if got.Name != m.metric || got.Unit != m.unit {
		t.Errorf("measurement = %s in %q, want %s in %q", got.Name, got.Unit, m.metric, m.unit)
	}
	if math.Abs(got.Value-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", m.metric, got.Value, want)
	}
	if got.Trust != environment.TrustOperator {
		t.Errorf("measurement trust = %q, want operator, so it can settle a required criterion", got.Trust)
	}
	if got.Source == "" {
		t.Error("measurement has no source")
	}
	for k, v := range map[string]any{"verifier": m.capID, "metric": m.metric, "unit": m.unit} {
		if res.StateDelta[k] != v {
			t.Errorf("state_delta[%s] = %v, want %v", k, res.StateDelta[k], v)
		}
	}
	if value, ok := res.StateDelta["value"].(float64); !ok || math.Abs(value-want) > 1e-9 {
		t.Errorf("state_delta[value] = %v, want %v", res.StateDelta["value"], want)
	}
}

// assertTelemetryInconclusive requires the inconclusive helper's shape: not a
// success, state inconclusive with the reason, and no measurement at all, so
// nothing can be read as a zero.
func assertTelemetryInconclusive(t *testing.T, res environment.ActionResult, capID string, reason objective.InconclusiveReason) {
	t.Helper()
	if res.Success {
		t.Fatalf("%s succeeded with %+v; what cannot be measured is inconclusive", capID, res.StateDelta)
	}
	if len(res.Measurements) != 0 {
		t.Errorf("%s measured %+v; an inconclusive verifier reports no number, not 0, -1 or NaN", capID, res.Measurements)
	}
	if got := res.StateDelta["state"]; got != string(objective.CriterionInconclusive) {
		t.Errorf("state = %v, want inconclusive", got)
	}
	if got := res.StateDelta["reason"]; got != string(reason) {
		t.Errorf("reason = %v, want %s (error %q)", got, reason, res.Error)
	}
	if res.StateDelta["verifier"] != capID {
		t.Errorf("verifier = %v, want %s", res.StateDelta["verifier"], capID)
	}
}

// cost_per_pass = Spend.Cost / Work.Reconciles: 3.00 USD over four passes.
func TestCostPerPassIsSpendOverReconciles(t *testing.T) {
	env := &telemetryEnv{reader: &fixedReader{snap: pricedSnapshot()}}
	res := actTelemetry(t, env, CapCostPerPass, nil)
	assertTelemetryMeasured(t, res, telemetryMetrics[0], 0.75)
}

// No pass in the window is no data: never 0, never NaN or +Inf from a
// division by zero.
func TestCostPerPassWithNoPassIsNoData(t *testing.T) {
	snap := pricedSnapshot()
	snap.Work.Reconciles = 0
	env := &telemetryEnv{reader: &fixedReader{snap: snap}}
	assertTelemetryInconclusive(t, actTelemetry(t, env, CapCostPerPass, nil), CapCostPerPass, objective.ReasonNoData)
}

// Unpriced spend is no data, never a zero cost. readSpend leaves Priced false
// both when no rate table prices the units and when the cost report could not
// be read, and the message has to say which kind of gap this is.
func TestCostPerPassWithUnpricedSpendIsNoData(t *testing.T) {
	for name, cost := range map[string]float64{"zero cost": 0, "stale cost": 3.0} {
		t.Run(name, func(t *testing.T) {
			snap := pricedSnapshot()
			snap.Spend = coretelemetry.SpendStats{Cost: cost, Priced: false}
			env := &telemetryEnv{reader: &fixedReader{snap: snap}}
			res := actTelemetry(t, env, CapCostPerPass, nil)
			assertTelemetryInconclusive(t, res, CapCostPerPass, objective.ReasonNoData)
			msg := strings.ToLower(res.Error)
			if !strings.Contains(msg, "rate table") && !strings.Contains(msg, "unread") {
				t.Errorf("error %q says neither that no rate table priced the spend nor that it was unread", res.Error)
			}
		})
	}
}

// escalation_approval_rate = EscalationStats.ApprovalRate(): three approvals
// and one rejection.
func TestEscalationApprovalRateIsApprovedOverResolved(t *testing.T) {
	env := &telemetryEnv{reader: &fixedReader{snap: pricedSnapshot()}}
	res := actTelemetry(t, env, CapEscalationApprovalRate, nil)
	assertTelemetryMeasured(t, res, telemetryMetrics[1], 0.75)
}

// Nothing resolved (ApprovalRate -1) is no data: never -1 and never 0, since
// "nobody decided" and "everything was rejected" are opposite signals
// (internal/core/telemetry/telemetry.go:77-82). Pending escalations do not
// count as resolved.
func TestEscalationApprovalRateWithNothingResolvedIsNoData(t *testing.T) {
	snap := pricedSnapshot()
	snap.Escalation = coretelemetry.EscalationStats{Escalations: 2, Pending: 2}
	env := &telemetryEnv{reader: &fixedReader{snap: snap}}
	assertTelemetryInconclusive(t, actTelemetry(t, env, CapEscalationApprovalRate, nil),
		CapEscalationApprovalRate, objective.ReasonNoData)
}

// A deployment with no telemetry reader cannot see: unsupported, never an
// empty success (AGENTS.md rule 10, ADR 026). A reader that fails is a
// verifier error.
func TestTelemetryMetricsBlindOrFailingAreInconclusive(t *testing.T) {
	for _, m := range telemetryMetrics {
		t.Run(m.metric+"/nil reader", func(t *testing.T) {
			env := &telemetryEnv{}
			assertTelemetryInconclusive(t, actTelemetry(t, env, m.capID, nil), m.capID, objective.ReasonUnsupported)
		})
		t.Run(m.metric+"/reader error", func(t *testing.T) {
			env := &telemetryEnv{reader: &fixedReader{err: errors.New("audit store unavailable")}}
			assertTelemetryInconclusive(t, actTelemetry(t, env, m.capID, nil), m.capID, objective.ReasonVerifierError)
		})
	}
}

// params.window is a Go duration that sets Query.Since, defaulting to
// telemetryWindow, the same rule as CapAnalyseUsage; the reader is asked for
// exactly that window.
func TestTelemetryMetricsApplyTheWindow(t *testing.T) {
	for _, m := range telemetryMetrics {
		for name, tc := range map[string]struct {
			params map[string]any
			want   time.Duration
		}{
			"default": {nil, telemetryWindow},
			"24h":     {map[string]any{"window": "24h"}, 24 * time.Hour},
		} {
			t.Run(m.metric+"/"+name, func(t *testing.T) {
				reader := &fixedReader{snap: pricedSnapshot()}
				env := &telemetryEnv{reader: reader}
				before := time.Now().UTC()
				res := actTelemetry(t, env, m.capID, tc.params)
				after := time.Now().UTC()
				if !res.Success {
					t.Fatalf("%s did not succeed: %s", m.capID, res.Error)
				}
				if reader.calls != 1 {
					t.Fatalf("reader asked %d times, want once", reader.calls)
				}
				lo, hi := before.Add(-tc.want), after.Add(-tc.want)
				if reader.last.Since.Before(lo) || reader.last.Since.After(hi) {
					t.Errorf("Query.Since = %v, want now - %v (between %v and %v)", reader.last.Since, tc.want, lo, hi)
				}
			})
		}
	}
}

// The environment's twin is a ceiling: params.twin_id cannot widen it to
// another tenant, the same rule as CapAnalyseUsage
// (TestAnalyseUsageCannotWidenBeyondItsTwin). A deployment-wide environment
// may still narrow to one twin.
func TestTelemetryMetricsCannotWidenBeyondTheirTwin(t *testing.T) {
	for _, m := range telemetryMetrics {
		t.Run(m.metric+"/ceiling", func(t *testing.T) {
			reader := &fixedReader{snap: pricedSnapshot()}
			env := &telemetryEnv{reader: reader, twinID: "twin-a"}
			actTelemetry(t, env, m.capID, map[string]any{"twin_id": "twin-b"})
			if reader.last.TwinID != "twin-a" {
				t.Errorf("Query.TwinID = %q, want twin-a: params.twin_id widened the environment's twin", reader.last.TwinID)
			}
		})
		t.Run(m.metric+"/narrow", func(t *testing.T) {
			reader := &fixedReader{snap: pricedSnapshot()}
			env := &telemetryEnv{reader: reader}
			actTelemetry(t, env, m.capID, map[string]any{"twin_id": "twin-b"})
			if reader.last.TwinID != "twin-b" {
				t.Errorf("Query.TwinID = %q, want twin-b", reader.last.TwinID)
			}
		})
	}
}

// Both verifiers are declared in the pack as verifiable, each with its metric,
// unit and direction, so create can check a measured criterion against them,
// and the telemetry factory serves both so stepAct routes them there.
func TestTelemetryMetricsAreDeclaredAndServed(t *testing.T) {
	declared := map[capability.CapabilityID]capability.Capability{}
	for _, c := range New().Capabilities() {
		declared[c.ID] = c
	}
	for _, m := range telemetryMetrics {
		t.Run(m.metric, func(t *testing.T) {
			c, ok := declared[capability.CapabilityID(m.capID)]
			if !ok {
				t.Fatalf("the software pack does not declare %s", m.capID)
			}
			if !c.Verifiable {
				t.Errorf("%s is not Verifiable", m.capID)
			}
			if len(c.Metrics) != 1 {
				t.Fatalf("%s declares metrics %+v, want exactly one", m.capID, c.Metrics)
			}
			got := c.Metrics[0]
			if got.Name != m.metric || got.Unit != m.unit || got.Direction != m.direction {
				t.Errorf("metric = {%s %s %s}, want {%s %s %s}", got.Name, got.Unit, got.Direction, m.metric, m.unit, m.direction)
			}
			// params.window is declared, so a criterion may carry it past create.
			window := false
			for _, p := range got.Params {
				window = window || (p.Name == "window" && p.Type == capability.ParamString)
			}
			if !window {
				t.Errorf("%s declares params %+v, want a string param window", m.metric, got.Params)
			}
			if env, ok := servedBy(capability.CapabilityID(m.capID)); !ok || env != EnvPlatformTelemetry {
				t.Errorf("%s is served by %q (found %v), want %s", m.capID, env, ok, EnvPlatformTelemetry)
			}
		})
	}

	serves := map[capability.CapabilityID]bool{}
	for _, id := range platformTelemetryFactory().Serves {
		serves[id] = true
	}
	for _, id := range []capability.CapabilityID{CapAnalyseUsage, CapCostPerPass, CapEscalationApprovalRate} {
		if !serves[id] {
			t.Errorf("platformTelemetryFactory().Serves = %v, missing %s", platformTelemetryFactory().Serves, id)
		}
	}
}

// Measuring did not make the environment writable: any other capability is
// still refused out loud, and the reader is never asked.
func TestTelemetryEnvStillRefusesEverythingElse(t *testing.T) {
	reader := &fixedReader{snap: pricedSnapshot()}
	env := &telemetryEnv{reader: reader}
	for _, id := range []string{"software.act.write_code", "software.act.create_pr", unservedVerifier} {
		res := actTelemetry(t, env, id, nil)
		if res.Success || !strings.Contains(res.Error, "read-only") {
			t.Errorf("%s: success %v, error %q; want a read-only refusal", id, res.Success, res.Error)
		}
	}
	if reader.calls != 0 {
		t.Errorf("reader asked %d times by refused capabilities, want 0", reader.calls)
	}
}

// unservedVerifier is a verifier this environment does not serve, spelled out
// so this test does not depend on Phase 41 step 12 landing first.
const unservedVerifier = "software.verify.p95_latency_ms"
