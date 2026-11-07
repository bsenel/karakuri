package loop

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 41 step 4: a measured criterion is a named number from the verifier
// it names, compared with its target by code. No model is asked about it.

const measuredVerifier = "software.verify.coverage"

// measuredContext is a step context with one required measured criterion,
// coverage >= 0.8 ratio, and a judge that would say PASS if it were asked.
func measuredContext() (*stepContext, *capturingAgent) {
	agent := &capturingAgent{reply: "PASS"}
	target := 0.8
	return &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-measured",
			SuccessCriteria: []objective.Criterion{{
				ID:          "coverage",
				Description: "coverage of the changed packages is at least 0.8",
				Kind:        objective.KindMeasured,
				Verifier:    measuredVerifier,
				Metric:      "coverage",
				Op:          objective.OpGreaterEqual,
				Target:      &target,
				Unit:        "ratio",
				Weight:      1.0,
			}},
		},
	}, agent
}

func coverage(value float64) environment.Measurement {
	return environment.Measurement{Name: "coverage", Value: value, Unit: "ratio", Source: measuredVerifier}
}

// The verifier's own measurement settles the criterion: 0.81 against >= 0.8
// is met and 0.79 is not, with no judge call either way.
func TestAMeasuredCriterionIsSettledByItsVerifiersMeasurement(t *testing.T) {
	cases := []struct {
		value float64
		want  objective.CriterionState
	}{
		{0.81, objective.CriterionMet},
		{0.79, objective.CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			sc, agent := measuredContext()

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
				CapabilityID: measuredVerifier,
				Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{coverage(tc.value)}},
			}})

			got := sc.obj.SuccessCriteria[0]
			if got.State != tc.want {
				t.Errorf("state = %q (reason %q), want %q", got.State, got.Reason, tc.want)
			}
			if allMet != (tc.want == objective.CriterionMet) {
				t.Errorf("allMet = %v with the criterion %q", allMet, got.State)
			}
			if agent.lastTask != "" {
				t.Error("the judge was asked about a measured criterion")
			}
		})
	}
}

// A verifier that ran and emitted no measurement gave no data: never a
// silent zero, never a pass (docs/roadmap.md:5473-5474, step 4 acceptance).
func TestAMeasuredCriterionWithNoMeasurementIsNoData(t *testing.T) {
	sc, agent := measuredContext()

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
		CapabilityID: measuredVerifier,
		Result:       environment.ActionResult{Success: true},
	}})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoData {
		t.Errorf("state = %q, reason = %q; want inconclusive: no_data", got.State, got.Reason)
	}
	if allMet {
		t.Error("a pass with no measurement completed")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}

// A verifier that cannot measure says so with ErrUnsupported, and that is
// inconclusive: unsupported; any other error is inconclusive: verifier_error
// (docs/roadmap.md:5547-5549, AGENTS.md rule 10). Both sentinels a verifier
// can meet today are covered: the forge's and the observability backend's.
func TestAMeasuredVerifierThatCannotMeasureIsInconclusive(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want objective.InconclusiveReason
	}{
		{"forge unsupported", fmt.Errorf("github: checks: %w", versioncontrol.ErrUnsupported), objective.ReasonUnsupported},
		{"observability unsupported", fmt.Errorf("loki: metrics: %w", observability.ErrUnsupported), objective.ReasonUnsupported},
		{"any other error", errors.New("coverage run: exit status 2"), objective.ReasonVerifierError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc, agent := measuredContext()

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
				CapabilityID: measuredVerifier,
				Result:       environment.ActionResult{Success: false, Error: tc.err.Error()},
				Errored:      true,
				Err:          tc.err,
			}})

			got := sc.obj.SuccessCriteria[0]
			if got.State != objective.CriterionInconclusive || got.Reason != tc.want {
				t.Errorf("state = %q, reason = %q; want inconclusive: %s", got.State, got.Reason, tc.want)
			}
			if allMet {
				t.Error("a verifier that could not measure completed the objective")
			}
			if agent.lastTask != "" {
				t.Error("the judge was asked about a measured criterion")
			}
		})
	}
}

// A measured verifier that ran and settled its own state inconclusive, as the
// runtime verifiers do (Phase 41 step 12), is read for its reason: unsupported
// and verifier_error stay what they are rather than becoming no_data.
func TestAMeasuredVerifierThatReportsItsReasonKeepsIt(t *testing.T) {
	for _, want := range []objective.InconclusiveReason{
		objective.ReasonUnsupported, objective.ReasonVerifierError, objective.ReasonNoData,
	} {
		t.Run(string(want), func(t *testing.T) {
			sc, agent := measuredContext()

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
				CapabilityID: measuredVerifier,
				Result: environment.ActionResult{Success: false, Error: "could not measure", StateDelta: map[string]any{
					"state": string(objective.CriterionInconclusive), "reason": string(want),
				}},
			}})

			got := sc.obj.SuccessCriteria[0]
			if got.State != objective.CriterionInconclusive || got.Reason != want {
				t.Errorf("state = %q, reason = %q; want inconclusive: %s", got.State, got.Reason, want)
			}
			if allMet {
				t.Error("a verifier that could not measure completed the objective")
			}
			if agent.lastTask != "" {
				t.Error("the judge was asked about a measured criterion")
			}
		})
	}
}

// A number read out of somebody else's text may be shown but cannot settle a
// required criterion (docs/roadmap.md:5541-5546, AGENTS.md rule 9): it is
// neither met, however good it is, nor not_met. The roadmap names no reason
// for this case, so only that there is one is checked (docs/roadmap.md:5470).
func TestAThirdPartyMeasurementCannotSettleARequiredCriterion(t *testing.T) {
	sc, agent := measuredContext()
	read := coverage(0.95)
	read.Source = "a figure in a pull request comment"
	read.Trust = environment.TrustThirdParty

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
		CapabilityID: measuredVerifier,
		Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{read}},
	}})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive {
		t.Errorf("state = %q, want inconclusive", got.State)
	}
	if got.Reason == "" {
		t.Error("inconclusive carries no reason")
	}
	if allMet {
		t.Error("a third-party measurement completed the objective")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}

// Only the named verifier settles the criterion: a measurement from any other
// capability is audited but settles nothing (docs/roadmap.md:5714-5717).
// Alone it leaves the criterion not_run; beside the verifier's own reading it
// does not change it.
func TestAMeasurementFromAnotherCapabilitySettlesNothing(t *testing.T) {
	other := actionOutcome{
		CapabilityID: "software.act.write_report",
		Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{coverage(0.95)}},
	}

	t.Run("alone", func(t *testing.T) {
		sc, agent := measuredContext()

		_, allMet := stepVerify(context.Background(), sc, []actionOutcome{other})

		got := sc.obj.SuccessCriteria[0]
		if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNotRun {
			t.Errorf("state = %q, reason = %q; want inconclusive: not_run", got.State, got.Reason)
		}
		if allMet || agent.lastTask != "" {
			t.Errorf("allMet = %v, judge asked = %v", allMet, agent.lastTask != "")
		}
	})

	t.Run("beside the verifier's own", func(t *testing.T) {
		sc, agent := measuredContext()

		_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
			other,
			{
				CapabilityID: measuredVerifier,
				Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{coverage(0.79)}},
			},
		})

		got := sc.obj.SuccessCriteria[0]
		if got.State != objective.CriterionNotMet {
			t.Errorf("state = %q (reason %q); another capability's 0.95 must not lift the verifier's 0.79", got.State, got.Reason)
		}
		if allMet || agent.lastTask != "" {
			t.Errorf("allMet = %v, judge asked = %v", allMet, agent.lastTask != "")
		}
	})
}
