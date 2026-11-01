package objective

import (
	"context"
	"math"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 8 part A: a criterion's sustain is checked at create time,
// beside the rest of step 5's validation, each refusal naming the field, and a
// request may not loosen a template's sustain (docs/roadmap.md, Phase 41
// section '4. Sustain for standing objectives').

func acceptanceSustain() *coreobjective.Sustain {
	return &coreobjective.Sustain{Window: "28d", Budget: 0.1, MinPasses: 3}
}

// sustainedTested is a valid verified criterion with step 8's sustain.
func sustainedTested(edit func(*coreobjective.Sustain)) coreobjective.Criterion {
	s := acceptanceSustain()
	edit(s)
	return coreobjective.Criterion{
		ID: "held", Description: "the tests keep passing", Kind: coreobjective.KindVerified,
		Verifier: testsVerifier, Sustain: s,
	}
}

// A sustain the reconcile loop can read is accepted, on any kind of criterion,
// and stored as written.
func TestCreateAcceptsASustain(t *testing.T) {
	for name, criterion := range map[string]coreobjective.Criterion{
		"28d, budget 0.1, min_passes 3": sustainedTested(func(*coreobjective.Sustain) {}),
		"a Go duration window":          sustainedTested(func(s *coreobjective.Sustain) { s.Window = "72h" }),
		"budget 0":                      sustainedTested(func(s *coreobjective.Sustain) { s.Budget = 0 }),
		"budget 1":                      sustainedTested(func(s *coreobjective.Sustain) { s.Budget = 1 }),
		"min_passes 0":                  sustainedTested(func(s *coreobjective.Sustain) { s.MinPasses = 0 }),
		"on a measured criterion": {
			ID: "latency-held", Description: "p95 latency holds", Kind: coreobjective.KindMeasured,
			Verifier: latencyVerifier, Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: target(200),
			Unit: "ms", Sustain: acceptanceSustain(),
		},
		"on a judged criterion": {
			ID: "reviewed", Description: "an open PR with tests", Kind: coreobjective.KindJudged,
			Evidence: "the open pull request", Sustain: acceptanceSustain(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			o, err := svc.Create(context.Background(), CreateRequest{
				Title: "standing", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{criterion},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := svc.Get(context.Background(), o.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			c := criterionByID(t, got.SuccessCriteria, criterion.ID)
			if c.Sustain == nil || *c.Sustain != *criterion.Sustain {
				t.Errorf("stored sustain = %+v, want %+v", c.Sustain, criterion.Sustain)
			}
		})
	}
}

// A sustain reconcile could never read the way its author meant is refused,
// naming the field.
func TestCreateRejectsAnInvalidSustain(t *testing.T) {
	for name, tc := range map[string]struct {
		criterion coreobjective.Criterion
		field     string
	}{
		"no window": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "" }),
			"success_criteria[1].sustain.window",
		},
		"an unparseable window": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "fortnight" }),
			"success_criteria[1].sustain.window",
		},
		"a window without a unit": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "28" }),
			"success_criteria[1].sustain.window",
		},
		"a zero window": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "0d" }),
			"success_criteria[1].sustain.window",
		},
		"a zero duration window": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "0s" }),
			"success_criteria[1].sustain.window",
		},
		"a negative window": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Window = "-72h" }),
			"success_criteria[1].sustain.window",
		},
		"a negative budget": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Budget = -0.1 }),
			"success_criteria[1].sustain.budget",
		},
		"a budget above 1": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Budget = 1.5 }),
			"success_criteria[1].sustain.budget",
		},
		"a NaN budget": {
			sustainedTested(func(s *coreobjective.Sustain) { s.Budget = math.NaN() }),
			"success_criteria[1].sustain.budget",
		},
		"a negative min_passes": {
			sustainedTested(func(s *coreobjective.Sustain) { s.MinPasses = -1 }),
			"success_criteria[1].sustain.min_passes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			// A valid criterion first, so the index names the criterion's own
			// position.
			assertRejected(t, svc, CreateRequest{
				Title: "standing", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{
					{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
					tc.criterion,
				},
			}, tc.field)
		})
	}
}

// sustainedTemplate holds p95 latency under 200 ms with step 8's sustain.
func sustainedTemplate() coreobjective.Template {
	return coreobjective.Template{ID: "tmpl.sustained", SuccessCriteria: []coreobjective.Criterion{{
		ID: "latency", Description: "p95 latency holds", Kind: coreobjective.KindMeasured, Verifier: latencyVerifier,
		Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: target(200), Unit: "ms",
		Sustain: acceptanceSustain(),
	}}}
}

func newSustainTemplateService(t *testing.T) *Service {
	t.Helper()
	svc := newCriteriaService(t)
	svc.RegisterTemplate(sustainedTemplate())
	return svc
}

// sustainedLatency is the template's criterion with its sustain edited.
func sustainedLatency(edit func(*coreobjective.Criterion)) coreobjective.Criterion {
	c := sustainedTemplate().SuccessCriteria[0]
	s := *c.Sustain
	c.Sustain = &s
	edit(&c)
	return c
}

// A request may tighten a template criterion's sustain: a smaller budget, a
// longer window or more passes before it says anything.
func TestCreateAcceptsATighterSustain(t *testing.T) {
	for name, tc := range map[string]struct {
		template  string
		tightened coreobjective.Criterion
	}{
		"the same sustain":    {"tmpl.sustained", sustainedLatency(func(*coreobjective.Criterion) {})},
		"a smaller budget":    {"tmpl.sustained", sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Budget = 0 })},
		"a longer window":     {"tmpl.sustained", sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Window = "56d" })},
		"the same window":     {"tmpl.sustained", sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Window = "672h" })},
		"a higher min_passes": {"tmpl.sustained", sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.MinPasses = 5 })},
		// The delivery template's latency has no sustain; holding it over
		// time asks more of it.
		"a sustain where the template had none": {"tmpl.delivery", func() coreobjective.Criterion {
			c := deliveryTemplate().SuccessCriteria[2]
			c.Sustain = acceptanceSustain()
			return c
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newSustainTemplateService(t)
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
			if c := criterionByID(t, got.SuccessCriteria, "latency"); c.Sustain == nil || *c.Sustain != *tc.tightened.Sustain {
				t.Errorf("stored sustain = %+v, want the request's %+v", c.Sustain, tc.tightened.Sustain)
			}
		})
	}
}

// A request may not loosen a template criterion's sustain. A larger budget
// forgives more failures, a shorter window forgets them sooner, and a lower
// min_passes reports sustained from fewer passes; each is refused, naming the
// field.
func TestCreateRejectsALooserSustain(t *testing.T) {
	for name, tc := range map[string]struct {
		loosened coreobjective.Criterion
		field    string
	}{
		"a larger budget": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Budget = 0.2 }),
			"success_criteria[0].sustain.budget",
		},
		"a shorter window": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Window = "14d" }),
			"success_criteria[0].sustain.window",
		},
		"a shorter Go duration window": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Window = "72h" }),
			"success_criteria[0].sustain.window",
		},
		"a lower min_passes": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.MinPasses = 1 }),
			"success_criteria[0].sustain.min_passes",
		},
		"no sustain": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain = nil }),
			"success_criteria[0].sustain",
		},
		// A tightening is still validated: an unreadable window is not
		// "longer".
		"an unparseable window": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Window = "forever" }),
			"success_criteria[0].sustain.window",
		},
		"a budget below 0": {
			sustainedLatency(func(c *coreobjective.Criterion) { c.Sustain.Budget = -1 }),
			"success_criteria[0].sustain.budget",
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newSustainTemplateService(t)
			assertRejected(t, svc, CreateRequest{
				Title: "hold", Domain: "software", TemplateID: "tmpl.sustained",
				SuccessCriteria: []coreobjective.Criterion{tc.loosened},
			}, tc.field)
		})
	}
}
