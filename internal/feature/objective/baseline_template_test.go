package objective

import (
	"context"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 7 part B: a request may change a template criterion's
// baseline or change only where it is plainly stricter, and a template's own
// number baseline is frozen like a request's.

// relativeTemplate holds p95 latency to at most 10% above a baseline of 200
// ms, so at most 220 ms.
func relativeTemplate() coreobjective.Template {
	return coreobjective.Template{ID: "tmpl.relative", SuccessCriteria: []coreobjective.Criterion{{
		ID: "latency", Description: "p95 latency holds", Kind: coreobjective.KindMeasured, Verifier: latencyVerifier,
		Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: target(10), Unit: "ms",
		Baseline: &coreobjective.Baseline{Value: 200}, Change: coreobjective.ChangePercent,
	}}}
}

func newBaselineTemplateService(t *testing.T) *Service {
	t.Helper()
	svc := newCriteriaService(t)
	svc.RegisterTemplate(relativeTemplate())
	return svc
}

func relativeLatencyAgainst(edit func(*coreobjective.Criterion)) coreobjective.Criterion {
	c := relativeTemplate().SuccessCriteria[0]
	edit(&c)
	return c
}

func TestCreateFreezesATemplatesNumberBaseline(t *testing.T) {
	svc := newBaselineTemplateService(t)
	o, err := svc.Create(context.Background(), CreateRequest{Title: "hold", Domain: "software", TemplateID: "tmpl.relative"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.Get(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c := criterionByID(t, got.SuccessCriteria, "latency"); c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Errorf("stored baseline_value = %v, want the template's 200", c.BaselineValue)
	}
}

func TestCreateAcceptsAPlainlyStricterBaseline(t *testing.T) {
	for name, tc := range map[string]struct {
		template  string
		tightened coreobjective.Criterion
		frozen    *float64
	}{
		// 10% above 150 is 165 ms, under the template's raw 200.
		"a relative target on a raw template": {"tmpl.delivery", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{Value: 150}
		}), target(150)},
		"the same baseline, a lower target": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Target = target(5)
		}), target(200)},
		// 10% above 180 is 198 ms, under 220.
		"a lower baseline": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{Value: 180}
		}), target(180)},
		// 200 + 15 is 215 ms, under 220.
		"absolute instead of percent": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Change, c.Target = coreobjective.ChangeAbsolute, target(15)
		}), target(200)},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newBaselineTemplateService(t)
			o, err := svc.Create(context.Background(), CreateRequest{
				Title: "hold", Domain: "software", TemplateID: tc.template,
				SuccessCriteria: []coreobjective.Criterion{tc.tightened},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := svc.Get(context.Background(), o.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			c := criterionByID(t, got.SuccessCriteria, "latency")
			if c.Change != tc.tightened.Change || c.BaselineValue == nil || *c.BaselineValue != *tc.frozen {
				t.Errorf("stored change %q, baseline_value %v; want %q and %v", c.Change, c.BaselineValue, tc.tightened.Change, *tc.frozen)
			}
		})
	}
}

func TestCreateRejectsALoosenedOrUncomparableBaseline(t *testing.T) {
	for name, tc := range map[string]struct {
		template string
		loosened coreobjective.Criterion
		field    string
	}{
		// 10% above 200 is 220 ms, over the template's raw 200.
		"a relative target looser than the raw one": {"tmpl.delivery", relativeLatencyAgainst(func(*coreobjective.Criterion) {}), "success_criteria[1].change"},
		// Nobody knows the start's number yet, so nobody can say it is stricter.
		"at_start on a raw template": {"tmpl.delivery", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{AtStart: true}
		}), "success_criteria[1].change"},
		"the same baseline, a higher target": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Target = target(20)
		}), "success_criteria[1].target"},
		// 10% above 250 is 275 ms, over 220.
		"a higher baseline": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{Value: 250}
		}), "success_criteria[1].baseline"},
		"at_start instead of a number": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline = &coreobjective.Baseline{AtStart: true}
		}), "success_criteria[1].baseline"},
		// A raw 230 is over 220.
		"the change dropped for a looser raw target": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Baseline, c.Change, c.Target = nil, "", target(230)
		}), "success_criteria[1].change"},
		"an unknown change": {"tmpl.relative", relativeLatencyAgainst(func(c *coreobjective.Criterion) {
			c.Change = "ratio"
		}), "success_criteria[1].change"},
		// A verified criterion has no value to set against a baseline.
		"a baseline on a verified template criterion": {"tmpl.delivery", coreobjective.Criterion{
			ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier,
			Baseline: &coreobjective.Baseline{Value: 1},
		}, "success_criteria[1].baseline"},
	} {
		t.Run(name, func(t *testing.T) {
			assertRejected(t, newBaselineTemplateService(t), CreateRequest{
				Title: "hold", Domain: "software", TemplateID: tc.template,
				SuccessCriteria: []coreobjective.Criterion{
					{ID: "reviewed", Description: "an open PR with tests", Kind: coreobjective.KindJudged, Evidence: "the PR"},
					tc.loosened,
				},
			}, tc.field)
		})
	}
}
