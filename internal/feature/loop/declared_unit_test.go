package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 4: a measured criterion's unit must equal the unit its
// verifier declares for the metric (docs/roadmap.md, Phase 41 decision 2,
// 'Declaration', and decision 3, 'Form'). The declaration is read where the
// loop reads every capability, the registry on the service; Phase 39's pack
// manifest is not on main, so it sits on capability.Capability itself.

// declaringContext is measuredContext, coverage >= 0.8 ratio, with the
// verifier registered declaring coverage in declaredUnit.
func declaringContext(t *testing.T, declaredUnit string) (*stepContext, *capturingAgent) {
	t.Helper()
	sc, agent := measuredContext()
	reg := capability.NewRegistry()
	if err := reg.Register(capability.Capability{
		ID:         measuredVerifier,
		Domain:     "software",
		Verifiable: true,
		Metrics: []capability.Metric{{
			Name:      "coverage",
			Unit:      declaredUnit,
			Direction: capability.DirectionHigherIsBetter,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	sc.svc.capReg = reg
	return sc, agent
}

// The verifier declares coverage in percent and the criterion asks for a
// ratio. Whatever unit the measurement then arrives in, the comparison is not
// one its author could have meant, so the criterion settles nothing: neither
// met nor not_met, inconclusive with a reason, and no judge. The roadmap names
// no reason for this case, so only that there is one is checked
// (docs/roadmap.md:5470).
func TestAMeasuredCriterionInAnotherUnitThanItsVerifierDeclaresSettlesNothing(t *testing.T) {
	cases := []struct {
		name string
		unit string
	}{
		// In the criterion's unit, so comparing it alone would settle it met.
		{"measured in the criterion's unit", "ratio"},
		{"measured in the declared unit", "percent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc, agent := declaringContext(t, "percent")
			m := coverage(0.95)
			m.Unit = tc.unit

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
				CapabilityID: measuredVerifier,
				Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{m}},
			}})

			got := sc.obj.SuccessCriteria[0]
			if got.State != objective.CriterionInconclusive {
				t.Errorf("state = %q (reason %q), want inconclusive", got.State, got.Reason)
			}
			if got.Reason == "" {
				t.Error("inconclusive carries no reason")
			}
			if allMet {
				t.Error("a criterion in another unit than its verifier declares completed the objective")
			}
			if agent.lastTask != "" {
				t.Error("the judge was asked about a measured criterion")
			}
		})
	}
}

// The same criterion against a verifier that declares coverage as a ratio is
// settled by the measurement: reading the declaration does not stop a
// criterion that agrees with it from settling.
func TestAMeasuredCriterionInItsVerifiersDeclaredUnitIsSettled(t *testing.T) {
	sc, agent := declaringContext(t, "ratio")

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
		CapabilityID: measuredVerifier,
		Result:       environment.ActionResult{Success: true, Measurements: []environment.Measurement{coverage(0.95)}},
	}})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionMet || !allMet {
		t.Errorf("state = %q (reason %q), allMet = %v; want met", got.State, got.Reason, allMet)
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a measured criterion")
	}
}
