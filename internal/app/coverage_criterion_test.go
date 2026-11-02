package app

import (
	"errors"
	"path/filepath"
	"testing"

	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/capability"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// docs/roadmap.md, Phase 41 step 10: "coverage of a fixture package against
// at_start". A criterion naming the software pack's coverage verifier, with a
// target relative to an at_start baseline, is accepted by the production
// criteria validator when it is given the verifiers the way the API gives them
// (internal/api/server.go: objSvc.WithVerifiers(capReg), capReg holding the
// software pack's capabilities as bootstrap registers them), and settles met
// when the measured coverage holds the baseline.
//
// The test lives here, in the wiring package, because internal/feature does not
// import domains.

func coverageCriterion() coreobjective.Criterion {
	zero := 0.0
	return coreobjective.Criterion{
		ID: "coverage-holds", Description: "coverage of the changed packages does not drop",
		Kind: coreobjective.KindMeasured, Verifier: "software.verify.coverage",
		Metric: "coverage", Op: coreobjective.OpGreaterEqual, Target: &zero, Unit: "percent",
		Params:   map[string]any{"packages": "./..."},
		Baseline: &coreobjective.Baseline{AtStart: true}, Change: coreobjective.ChangeAbsolute,
	}
}

func objectiveServiceWithThePacksVerifiers(t *testing.T) *featureobjective.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	capReg := capability.NewRegistry()
	for _, c := range domainsw.New().Capabilities() {
		if err := capReg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	return featureobjective.NewService(storage.NewGORMStorage(db)).WithVerifiers(capReg)
}

func TestACoverageCriterionAgainstAtStartIsAcceptedAndSettlesMet(t *testing.T) {
	svc := objectiveServiceWithThePacksVerifiers(t)
	created, err := svc.Create(t.Context(), featureobjective.CreateRequest{
		Title: "keep coverage", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{coverageCriterion()},
	})
	if err != nil {
		t.Fatalf("the production validator refused a coverage criterion against at_start: %v", err)
	}
	if len(created.SuccessCriteria) != 1 {
		t.Fatalf("created criteria = %#v, want the one given", created.SuccessCriteria)
	}
	c := created.SuccessCriteria[0]
	if c.BaselineValue != nil {
		t.Fatalf("an at_start baseline was frozen at create to %v; it is measured when the objective starts", *c.BaselineValue)
	}

	// at_start measured 50; a pass that measures 50 has not dropped it.
	baseline := 50.0
	c.BaselineValue = &baseline
	if state, reason := c.Compare(50, "percent"); state != coreobjective.CriterionMet {
		t.Errorf("baseline 50, measured 50, change >= 0: state %s (%s), want met", state, reason)
	}
	if state, _ := c.Compare(49.5, "percent"); state != coreobjective.CriterionNotMet {
		t.Errorf("baseline 50, measured 49.5, change >= 0: state %s, want not_met", state)
	}
}

// The validator reads the pack's declaration, so a criterion it does not
// declare is refused naming the field: another unit, another metric, or a
// parameter the coverage metric does not take.
func TestACoverageCriterionOutsideTheDeclarationIsRefused(t *testing.T) {
	svc := objectiveServiceWithThePacksVerifiers(t)
	for field, mutate := range map[string]func(*coreobjective.Criterion){
		"success_criteria[0].unit":          func(c *coreobjective.Criterion) { c.Unit = "ratio" },
		"success_criteria[0].metric":        func(c *coreobjective.Criterion) { c.Metric = "flaky_test_count" },
		"success_criteria[0].params.branch": func(c *coreobjective.Criterion) { c.Params["branch"] = "main" },
	} {
		t.Run(field, func(t *testing.T) {
			c := coverageCriterion()
			mutate(&c)
			_, err := svc.Create(t.Context(), featureobjective.CreateRequest{
				Title: "keep coverage", Domain: "software", SuccessCriteria: []coreobjective.Criterion{c},
			})
			var verr *featureobjective.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Create = %v, want a *ValidationError", err)
			}
			if verr.Field != field {
				t.Errorf("refused field %q, want %q", verr.Field, field)
			}
		})
	}
}
