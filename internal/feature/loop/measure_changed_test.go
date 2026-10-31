package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// docs/roadmap.md, Phase 41 step 9 (decision 6, 'Anti-gaming'): a verifier
// declares the paths its definition is read from, and a pass that changes one
// of them makes a criterion naming that verifier inconclusive: measure_changed,
// waiting for a person, whatever the verifier then reported.
//
// The changed files of the pass come from the one source the pack already
// reads them from, the forge's changed files of the stream's pull request
// (software.verify.paths_declared, step 2). The named verifier's run carries
// them in StateDelta "changed_files"; the loop reads them there and compares
// them with the verifier's declared MeasurePaths from the registry, where it
// reads every declaration (step 4's Metrics). No second source is read.

const measureVerifier = "software.verify.run_tests"

// measureContext is one required verified criterion naming run_tests, with
// run_tests registered declaring paths as its measure paths, and a judge that
// would say PASS if it were asked.
func measureContext(t *testing.T, paths []string) (*stepContext, *capturingAgent) {
	t.Helper()
	reg := capability.NewRegistry()
	if err := reg.Register(capability.Capability{
		ID:           measureVerifier,
		Domain:       "software",
		Verifiable:   true,
		MeasurePaths: paths,
	}); err != nil {
		t.Fatal(err)
	}
	agent := &capturingAgent{reply: "PASS"}
	return &stepContext{
		svc:   &serviceImpl{hub: event.NewHub(), capReg: reg},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-measure-changed",
			SuccessCriteria: []objective.Criterion{{
				ID:          "tests-pass",
				Description: "All tests pass",
				Verifier:    measureVerifier,
				Weight:      1.0,
			}},
		},
	}, agent
}

// testsPassed is a successful run_tests whose pass changed files.
func testsPassed(files ...string) actionOutcome {
	return actionOutcome{
		CapabilityID: measureVerifier,
		Result: environment.ActionResult{
			Success:    true,
			StateDelta: map[string]any{"verifier": measureVerifier, "changed_files": files},
		},
	}
}

var goTestPaths = []string{"**/*_test.go"}

// The acceptance clause: a pass that edits a declared test file leaves the
// criterion inconclusive: measure_changed although run_tests succeeded. It is
// not met, so it completes nothing, and not not_met, so reconcile's breaker,
// which counts not_met passes only, does not count it either. No judge is asked.
func TestAPassThatEditsADeclaredTestFileIsMeasureChanged(t *testing.T) {
	sc, agent := measureContext(t, goTestPaths)

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		testsPassed("internal/feature/loop/verify.go", "internal/feature/loop/verify_test.go"),
	})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonMeasureChanged {
		t.Errorf("state = %q (reason %q), want inconclusive: measure_changed", got.State, got.Reason)
	}
	if allMet {
		t.Error("a pass that changed its own measure completed the objective")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a criterion whose measure changed")
	}
}

// A pass whose changed files miss the declared paths is settled as before: a
// successful run_tests is met, a failed one not_met.
func TestAPassThatMissesTheDeclaredPathsIsSettledAsBefore(t *testing.T) {
	for _, tc := range []struct {
		name    string
		success bool
		want    objective.CriterionState
	}{
		{"tests passed", true, objective.CriterionMet},
		{"tests failed", false, objective.CriterionNotMet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc, _ := measureContext(t, goTestPaths)
			run := testsPassed("internal/feature/loop/verify.go", "docs/roadmap.md")
			run.Result.Success = tc.success

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{run})

			got := sc.obj.SuccessCriteria[0]
			if got.State != tc.want || got.Reason != "" {
				t.Errorf("state = %q (reason %q), want %q", got.State, got.Reason, tc.want)
			}
			if allMet != (tc.want == objective.CriterionMet) {
				t.Errorf("allMet = %v with the criterion %q", allMet, got.State)
			}
		})
	}
}

// A pass that changed nothing has touched no declared path either.
func TestAPassThatChangedNoFilesIsSettledAsBefore(t *testing.T) {
	sc, _ := measureContext(t, goTestPaths)

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{testsPassed()})

	if got := sc.obj.SuccessCriteria[0]; got.State != objective.CriterionMet || !allMet {
		t.Errorf("state = %q (reason %q), allMet = %v; want met", got.State, got.Reason, allMet)
	}
}

// A verifier that declares no measure paths is never measure_changed, whatever
// the pass changed: there is nothing its definition is declared to live in.
// Nor is one the registry does not hold.
func TestAVerifierWithNoDeclaredPathsIsNeverMeasureChanged(t *testing.T) {
	for _, tc := range []struct {
		name       string
		unregister bool
	}{
		{"declares none", false},
		{"not in the registry", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc, _ := measureContext(t, nil)
			if tc.unregister {
				sc.svc.capReg = capability.NewRegistry()
			}

			_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
				testsPassed("internal/feature/loop/verify_test.go"),
			})

			if got := sc.obj.SuccessCriteria[0]; got.State != objective.CriterionMet || !allMet {
				t.Errorf("state = %q (reason %q), allMet = %v; want met", got.State, got.Reason, allMet)
			}
		})
	}
}

// When the changed files of the pass cannot be read, the run of a verifier that
// declares measure paths does not carry them, and the criterion is not
// silently met (AGENTS.md rule 10: a source that cannot see says so, and an
// absent signal is not an empty one). It is inconclusive: verifier_error, the
// reason verify already gives a verifier that could not report what it had to
// (qualifyingSamples, settleByVerifier). Not not_met, so it counts against
// nothing, and no judge is asked.
func TestUnreadableChangedFilesAreNotSilentlyMet(t *testing.T) {
	sc, agent := measureContext(t, goTestPaths)

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
		CapabilityID: measureVerifier,
		Result: environment.ActionResult{
			Success:    true,
			StateDelta: map[string]any{"verifier": measureVerifier},
		},
	}})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonVerifierError {
		t.Errorf("state = %q (reason %q), want inconclusive: verifier_error", got.State, got.Reason)
	}
	if allMet {
		t.Error("a pass whose changed files could not be read completed the objective")
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked")
	}
}

// The same holds for a measured criterion: a pass that edits the benchmark its
// verifier declares is measure_changed, however good the number it then
// measured.
func TestAMeasuredCriterionWhoseMeasureChangedIsMeasureChanged(t *testing.T) {
	sc, _ := measuredContext()
	reg := capability.NewRegistry()
	if err := reg.Register(capability.Capability{
		ID:           measuredVerifier,
		Domain:       "software",
		Verifiable:   true,
		Metrics:      []capability.Metric{{Name: "coverage", Unit: "ratio", Direction: capability.DirectionHigherIsBetter}},
		MeasurePaths: []string{".coveragerc"},
	}); err != nil {
		t.Fatal(err)
	}
	sc.svc.capReg = reg

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{{
		CapabilityID: measuredVerifier,
		Result: environment.ActionResult{
			Success:      true,
			StateDelta:   map[string]any{"changed_files": []string{".coveragerc"}},
			Measurements: []environment.Measurement{coverage(0.95)},
		},
	}})

	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonMeasureChanged {
		t.Errorf("state = %q (reason %q), want inconclusive: measure_changed", got.State, got.Reason)
	}
	if allMet {
		t.Error("a measured criterion whose measure changed completed the objective")
	}
}
