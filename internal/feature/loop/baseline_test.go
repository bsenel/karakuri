package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 7 part B: a measured criterion with a change is settled
// against its frozen baseline, and with no baseline it is inconclusive:
// no_baseline, never not_met (docs/roadmap.md, Phase 41 decision 3
// 'Baselines').

const latencyVerifier = "software.verify.latency"

// baselineContext is a step context with one required measured criterion,
// p95 latency not more than 10% above its baseline, and a judge that would say
// PASS if it were asked. A nil frozen value is an at_start baseline never
// measured.
func baselineContext(frozen *float64) (*stepContext, *capturingAgent) {
	agent := &capturingAgent{reply: "PASS"}
	target := 10.0
	baseline := &objective.Baseline{AtStart: true}
	if frozen != nil {
		baseline = &objective.Baseline{Value: *frozen}
	}
	return &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-baseline",
			SuccessCriteria: []objective.Criterion{{
				ID:            "latency",
				Description:   "p95 latency is not more than 10% above the baseline",
				Kind:          objective.KindMeasured,
				Verifier:      latencyVerifier,
				Metric:        "p95_latency_ms",
				Op:            objective.OpLessEqual,
				Target:        &target,
				Unit:          "ms",
				Weight:        1.0,
				Baseline:      baseline,
				Change:        objective.ChangePercent,
				BaselineValue: frozen,
			}},
		},
	}, agent
}

func latencyRun(value float64) []actionOutcome {
	return []actionOutcome{{
		CapabilityID: latencyVerifier,
		Result: environment.ActionResult{Success: true, Measurements: []environment.Measurement{
			{Name: "p95_latency_ms", Value: value, Unit: "ms", Source: latencyVerifier},
		}},
	}}
}

// A baseline of 200 fixed by hand: 215 is 7.5% above it and met, 230 is 15%
// above and not met. Compared raw, both would be far above 10 ms.
func TestAChangeIsSettledAgainstAFixedBaseline(t *testing.T) {
	for _, tc := range []struct {
		value  float64
		want   objective.CriterionState
		allMet bool
	}{
		{215, objective.CriterionMet, true},
		{230, objective.CriterionNotMet, false},
	} {
		frozen := 200.0
		sc, agent := baselineContext(&frozen)

		_, allMet := stepVerify(context.Background(), sc, latencyRun(tc.value))

		got := sc.obj.SuccessCriteria[0]
		if got.State != tc.want || got.Reason != "" {
			t.Errorf("p95 %v against 200 +10%%: state = %q (reason %q), want %q", tc.value, got.State, got.Reason, tc.want)
		}
		if allMet != tc.allMet {
			t.Errorf("p95 %v against 200 +10%%: completed = %v, want %v", tc.value, allMet, tc.allMet)
		}
		if agent.lastTask != "" {
			t.Error("the judge was asked about a measured criterion")
		}
	}
}

// The acceptance case: an at_start baseline that was never measured leaves a
// relative target inconclusive: no_baseline, not not_met, however good or bad
// the measurement. It blocks completion, and the state and reason are what the
// verify step records.
func TestAChangeWithNoBaselineIsInconclusiveNoBaseline(t *testing.T) {
	for _, value := range []float64{0, 150, 230, 1e6} {
		hub := event.NewHub()
		sc, agent := baselineContext(nil)
		sc.svc = &serviceImpl{hub: hub}
		log := collectEvents(hub, sc.obj.ID)

		_, allMet := stepVerify(context.Background(), sc, latencyRun(value))
		events := log.stop()

		got := sc.obj.SuccessCriteria[0]
		if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoBaseline {
			t.Errorf("p95 %v with no baseline: state = %q, reason = %q; want inconclusive: no_baseline", value, got.State, got.Reason)
		}
		if allMet {
			t.Errorf("p95 %v with no baseline completed the objective", value)
		}
		if agent.lastTask != "" {
			t.Error("the judge was asked about a measured criterion")
		}

		payloads := verifyCompleted(events)
		if len(payloads) != 1 {
			t.Fatalf("verify completed %d times, want once", len(payloads))
		}
		recorded, ok := payloads[0]["criteria"].([]objective.Criterion)
		if !ok {
			t.Fatalf("verify payload carries no criteria states: %#v", payloads[0])
		}
		if c := criterionByID(t, recorded, "latency"); c.State != objective.CriterionInconclusive || c.Reason != objective.ReasonNoBaseline {
			t.Errorf("recorded latency: state = %q, reason = %q; want inconclusive/no_baseline", c.State, c.Reason)
		}
	}
}

// A measured criterion with neither baseline nor change is settled on the raw
// value as before: 9 ms is met against <= 10.
func TestACriterionWithoutABaselineIsSettledAsBefore(t *testing.T) {
	sc, _ := baselineContext(nil)
	c := &sc.obj.SuccessCriteria[0]
	c.Baseline, c.Change, c.BaselineValue = nil, "", nil

	_, allMet := stepVerify(context.Background(), sc, latencyRun(9))

	if got := sc.obj.SuccessCriteria[0]; got.State != objective.CriterionMet || !allMet {
		t.Errorf("9 <= 10 with no baseline: state = %q (reason %q), completed = %v; want met and completed", got.State, got.Reason, allMet)
	}
}
