package objective

import (
	"context"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 7 part B: baseline and change are checked at create time, and
// a baseline given as a number is frozen at once (docs/roadmap.md, Phase 41
// decision 3 'Baselines').

// relativeLatency is a valid measured criterion: p95 latency not more than 10%
// above a baseline of 200 ms fixed by hand.
func relativeLatency(edit func(*coreobjective.Criterion)) coreobjective.Criterion {
	c := coreobjective.Criterion{
		ID: "latency-relative", Description: "p95 latency is not more than 10% above the baseline",
		Kind: coreobjective.KindMeasured, Verifier: latencyVerifier, Metric: "p95_latency_ms",
		Op: coreobjective.OpLessEqual, Target: target(10), Unit: "ms",
		Baseline: &coreobjective.Baseline{Value: 200}, Change: coreobjective.ChangePercent,
	}
	edit(&c)
	return c
}

func createOne(t *testing.T, criterion coreobjective.Criterion) coreobjective.Criterion {
	t.Helper()
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "plain", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{criterion},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.Get(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return criterionByID(t, got.SuccessCriteria, criterion.ID)
}

// A number given at create time is the frozen baseline at once.
func TestCreateFreezesABaselineGivenAsANumber(t *testing.T) {
	for name, criterion := range map[string]coreobjective.Criterion{
		"percent against 200": relativeLatency(func(*coreobjective.Criterion) {}),
		"absolute against 200": relativeLatency(func(c *coreobjective.Criterion) {
			c.Change, c.Target = coreobjective.ChangeAbsolute, target(20)
		}),
		// A request cannot write the frozen value itself; the number it gave is it.
		"a request's baseline_value is not kept": relativeLatency(func(c *coreobjective.Criterion) {
			c.BaselineValue = target(1e6)
		}),
	} {
		t.Run(name, func(t *testing.T) {
			c := createOne(t, criterion)
			if c.Baseline == nil || c.Baseline.AtStart || c.Baseline.Value != 200 {
				t.Errorf("stored baseline = %+v, want the number 200", c.Baseline)
			}
			if c.Change != criterion.Change {
				t.Errorf("stored change = %q, want %q", c.Change, criterion.Change)
			}
			if c.BaselineValue == nil || *c.BaselineValue != 200 {
				t.Errorf("stored baseline_value = %v, want 200, frozen at create time", c.BaselineValue)
			}
		})
	}
}

// at_start is measured when the objective starts (part C), so at create time
// there is no frozen value yet, whatever the request sent.
func TestCreateLeavesAnAtStartBaselineUnmeasured(t *testing.T) {
	for name, criterion := range map[string]coreobjective.Criterion{
		"at_start": relativeLatency(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{AtStart: true}
		}),
		"at_start with a baseline_value in the request": relativeLatency(func(c *coreobjective.Criterion) {
			c.Baseline, c.BaselineValue = &coreobjective.Baseline{AtStart: true}, target(1e6)
		}),
	} {
		t.Run(name, func(t *testing.T) {
			c := createOne(t, criterion)
			if c.Baseline == nil || !c.Baseline.AtStart {
				t.Errorf("stored baseline = %+v, want at_start", c.Baseline)
			}
			if c.BaselineValue != nil {
				t.Errorf("stored baseline_value = %v, want none until the start is measured", *c.BaselineValue)
			}
		})
	}
}

// A criterion without baseline and change is stored as before, with none.
func TestCreateKeepsACriterionWithoutABaselineAsBefore(t *testing.T) {
	c := createOne(t, relativeLatency(func(c *coreobjective.Criterion) {
		c.Baseline, c.Change, c.Target = nil, "", target(200)
	}))
	if c.Baseline != nil || c.Change != "" || c.BaselineValue != nil {
		t.Errorf("stored baseline %+v, change %q, baseline_value %v; want none", c.Baseline, c.Change, c.BaselineValue)
	}
}

// A baseline or change the loop could never compare with is refused, naming
// the field.
func TestCreateRejectsAnInvalidBaseline(t *testing.T) {
	for name, tc := range map[string]struct {
		criterion coreobjective.Criterion
		field     string
	}{
		// A change says how a value is set against a baseline; with none there
		// is nothing to set it against, ever.
		"change without a baseline": {
			relativeLatency(func(c *coreobjective.Criterion) { c.Baseline = nil }),
			"success_criteria[1].baseline",
		},
		"change relative": {
			relativeLatency(func(c *coreobjective.Criterion) { c.Change = "relative" }),
			"success_criteria[1].change",
		},
		"change ratio": {
			relativeLatency(func(c *coreobjective.Criterion) { c.Change = "ratio" }),
			"success_criteria[1].change",
		},
		// Only a measured criterion has a value to set against a baseline.
		"baseline on a verified criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: testsVerifier,
				Baseline: &coreobjective.Baseline{AtStart: true}},
			"success_criteria[1].baseline",
		},
		"change on a verified criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: testsVerifier,
				Change: coreobjective.ChangeAbsolute},
			"success_criteria[1].change",
		},
		"baseline on a judged criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindJudged, Evidence: "the PR",
				Baseline: &coreobjective.Baseline{Value: 0.8}},
			"success_criteria[1].baseline",
		},
		"change on a judged criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindJudged, Evidence: "the PR",
				Change: coreobjective.ChangePercent},
			"success_criteria[1].change",
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			// A valid criterion first, so the index names the criterion's own
			// position.
			assertRejected(t, svc, CreateRequest{
				Title: "plain", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{
					{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
					tc.criterion,
				},
			}, tc.field)
		})
	}
}
