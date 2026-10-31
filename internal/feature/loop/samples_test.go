package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 7 part A: a measured criterion over several samples is settled
// by its reducer, from the named verifier's own measurements, and not below
// min_samples (docs/roadmap.md, Phase 41 decision 3 'Repeated runs' and step 7).

const sampledVerifier = "software.verify.cli_startup"

// sampledContext is a step context with one required measured criterion,
// startup_ms <= 15 ms over three samples reduced by the median, and a judge
// that would say PASS if it were asked.
func sampledContext(minSamples int) (*stepContext, *capturingAgent) {
	agent := &capturingAgent{reply: "PASS"}
	target := 15.0
	return &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-sampled",
			SuccessCriteria: []objective.Criterion{{
				ID:          "startup",
				Description: "the CLI starts in 15 ms or less",
				Kind:        objective.KindMeasured,
				Verifier:    sampledVerifier,
				Metric:      "startup_ms",
				Op:          objective.OpLessEqual,
				Target:      &target,
				Unit:        "ms",
				Weight:      1.0,
				Samples:     3,
				Reducer:     objective.ReducerMedian,
				MinSamples:  minSamples,
			}},
		},
	}, agent
}

func startup(value float64) environment.Measurement {
	return environment.Measurement{Name: "startup_ms", Value: value, Unit: "ms", Source: sampledVerifier}
}

// runsOf is one verifier run per measurement, as repeated runs arrive.
func runsOf(ms ...environment.Measurement) []actionOutcome {
	out := make([]actionOutcome, 0, len(ms))
	for _, m := range ms {
		out = append(out, actionOutcome{
			CapabilityID: sampledVerifier,
			Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{m}},
		})
	}
	return out
}

// The roadmap's acceptance: the median of [10, 50, 12] against <= 15 is met.
// The 50 alone is not_met, so a verify step that settles on any one sample
// failing, or on the mean (24), gets this wrong.
func TestSampledCriterionIsSettledByTheMedianOfItsRuns(t *testing.T) {
	sc, agent := sampledContext(0)

	_, allMet := stepVerify(context.Background(), sc, runsOf(startup(10), startup(50), startup(12)))

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionMet {
		t.Errorf("state = %q (reason %q), want met by the median 12", got.State, got.Reason)
	}
	if !allMet {
		t.Error("the only required criterion is met and the objective did not complete")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}

// Two samples with min_samples: 3 are too_few_samples, however good they are,
// and the objective does not complete.
func TestSampledCriterionBelowMinSamplesIsTooFewSamples(t *testing.T) {
	sc, agent := sampledContext(3)

	_, allMet := stepVerify(context.Background(), sc, runsOf(startup(10), startup(12)))

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonTooFewSamples {
		t.Errorf("state = %q, reason = %q; want inconclusive: too_few_samples", got.State, got.Reason)
	}
	if allMet {
		t.Error("two samples with min_samples 3 completed the objective")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}

// A number read out of somebody else's text still cannot settle a required
// criterion (AGENTS.md rule 9), and it is not a sample either: two of the
// verifier's own and one third-party reading are two samples, below
// min_samples: 3.
func TestAThirdPartyMeasurementIsNotASample(t *testing.T) {
	sc, agent := sampledContext(3)
	read := startup(11)
	read.Source = "a figure in a pull request comment"
	read.Trust = environment.TrustThirdParty

	_, allMet := stepVerify(context.Background(), sc, runsOf(startup(10), read, startup(12)))

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonTooFewSamples {
		t.Errorf("state = %q, reason = %q; want inconclusive: too_few_samples, the third-party reading not counted", got.State, got.Reason)
	}
	if allMet {
		t.Error("a third-party measurement completed the objective")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}
