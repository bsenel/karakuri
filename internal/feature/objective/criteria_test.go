package objective

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 Step 5: criteria in the create request, merged with the template's
// and validated against the verifiers they name before anything is saved.

const (
	coverageVerifier capability.CapabilityID = "software.verify.coverage"
	latencyVerifier  capability.CapabilityID = "software.verify.latency"
	testsVerifier    capability.CapabilityID = "software.verify.run_tests"
)

// fakeVerifiers is a verifier lookup holding exactly the capabilities a test
// gives it.
type fakeVerifiers map[capability.CapabilityID]capability.Capability

func (f fakeVerifiers) Get(id capability.CapabilityID) (capability.Capability, bool) {
	c, ok := f[id]
	return c, ok
}

func testVerifiers() fakeVerifiers {
	discovered := capability.NewMCPCapability("github", "merge_check", "says whether a PR can merge", capability.Schema{})
	return fakeVerifiers{
		coverageVerifier: {ID: coverageVerifier, Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "coverage", Unit: "percent", Direction: capability.DirectionHigherIsBetter,
			Params: []capability.MetricParam{{Name: "packages", Type: capability.ParamString}},
		}}},
		latencyVerifier: {ID: latencyVerifier, Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "p95_latency_ms", Unit: "ms", Direction: capability.DirectionLowerIsBetter,
		}}},
		testsVerifier: {ID: testsVerifier, Domain: "software", Verifiable: true},
		discovered.ID: discovered,
	}
}

func target(v float64) *float64 { return &v }

func required(v bool) *bool { return &v }

// A template with one criterion of each kind, the measured ones optional so a
// request can tighten them to required.
func deliveryTemplate() coreobjective.Template {
	return coreobjective.Template{ID: "tmpl.delivery", SuccessCriteria: []coreobjective.Criterion{
		{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
		{
			ID: "coverage", Description: "coverage holds", Kind: coreobjective.KindMeasured, Verifier: coverageVerifier,
			Metric: "coverage", Op: coreobjective.OpGreaterEqual, Target: target(80), Unit: "percent", Required: required(false),
		},
		{
			ID: "latency", Description: "p95 latency holds", Kind: coreobjective.KindMeasured, Verifier: latencyVerifier,
			Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: target(200), Unit: "ms", Required: required(false),
		},
	}}
}

func newCriteriaService(t *testing.T) *Service {
	t.Helper()
	svc := newTestService(t).WithVerifiers(testVerifiers())
	svc.RegisterTemplate(deliveryTemplate())
	return svc
}

func criterionIDs(cs []coreobjective.Criterion) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func criterionByID(t *testing.T, cs []coreobjective.Criterion, id string) coreobjective.Criterion {
	t.Helper()
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no criterion %q in %v", id, criterionIDs(cs))
	return coreobjective.Criterion{}
}

// assertRejected checks that create refused the request for the named field and
// saved nothing.
func assertRejected(t *testing.T, svc *Service, req CreateRequest, field string) {
	t.Helper()
	before, err := svc.List(context.Background(), storage.ObjectiveFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	_, err = svc.Create(context.Background(), req)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a *ValidationError naming %s", err, field)
	}
	if verr.Field != field {
		t.Errorf("rejected field = %q, want %q (err: %v)", verr.Field, field, err)
	}
	after, err := svc.List(context.Background(), storage.ObjectiveFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("a rejected objective was saved anyway")
	}
}

// Request criteria come after the template's, which stay the default, and both
// reach the store with every field the request gave.
func TestCreateAddsRequestCriteriaAfterTheTemplates(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "ship it", Domain: "software", TemplateID: "tmpl.delivery",
		SuccessCriteria: []coreobjective.Criterion{
			{
				ID: "reviewed", Description: "an open PR with tests", Kind: coreobjective.KindJudged,
				Evidence: "the pull request's diff and its test files",
			},
			{
				ID: "core-coverage", Description: "core stays covered", Kind: coreobjective.KindMeasured,
				Verifier: coverageVerifier, Metric: "coverage", Op: coreobjective.OpGreaterEqual, Target: target(90),
				Unit: "percent", Params: map[string]any{"packages": "./internal/core/..."},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	want := []string{"tested", "coverage", "latency", "reviewed", "core-coverage"}
	got, err := svc.Get(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ids := criterionIDs(got.SuccessCriteria); !slices.Equal(ids, want) {
		t.Fatalf("stored criteria = %v, want %v", ids, want)
	}
	if e := criterionByID(t, got.SuccessCriteria, "reviewed").Evidence; e != "the pull request's diff and its test files" {
		t.Errorf("stored evidence = %q, want the request's", e)
	}
	if p := criterionByID(t, got.SuccessCriteria, "core-coverage").Params["packages"]; p != "./internal/core/..." {
		t.Errorf("stored params[packages] = %v, want the request's", p)
	}
}

// Without a template, the request's criteria are the objective's.
func TestCreateTakesRequestCriteriaWithoutATemplate(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "plain", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{
			{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ids := criterionIDs(o.SuccessCriteria); !slices.Equal(ids, []string{"tested"}) {
		t.Errorf("criteria = %v, want [tested]", ids)
	}
}

// A request criterion with a template criterion's ID replaces it in place when
// it only tightens it: a stricter target in the direction of its op, or
// required where the template had it optional.
func TestCreateAcceptsATightenedTemplateCriterion(t *testing.T) {
	tmpl := deliveryTemplate()
	coverage, latency := tmpl.SuccessCriteria[1], tmpl.SuccessCriteria[2]

	higherCoverage := coverage
	higherCoverage.Target = target(90)
	lowerLatency := latency
	lowerLatency.Target = target(150)
	requiredCoverage := coverage
	requiredCoverage.Required = required(true)

	for name, tc := range map[string]struct {
		tightened coreobjective.Criterion
		check     func(coreobjective.Criterion) bool
	}{
		"higher target on >=":  {higherCoverage, func(c coreobjective.Criterion) bool { return c.Target != nil && *c.Target == 90 }},
		"lower target on <=":   {lowerLatency, func(c coreobjective.Criterion) bool { return c.Target != nil && *c.Target == 150 }},
		"optional to required": {requiredCoverage, func(c coreobjective.Criterion) bool { return c.IsRequired() }},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			o, err := svc.Create(context.Background(), CreateRequest{
				Title: "ship it", Domain: "software", TemplateID: "tmpl.delivery",
				SuccessCriteria: []coreobjective.Criterion{tc.tightened},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := svc.Get(context.Background(), o.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			// Replaced where the template put it, not added a second time.
			if ids := criterionIDs(got.SuccessCriteria); !slices.Equal(ids, []string{"tested", "coverage", "latency"}) {
				t.Fatalf("criteria = %v, want the template's three with %s tightened in place", ids, tc.tightened.ID)
			}
			if c := criterionByID(t, got.SuccessCriteria, tc.tightened.ID); !tc.check(c) {
				t.Errorf("stored %s = %+v, want the tightened one", c.ID, c)
			}
		})
	}
}

// Anything else a request does to a template criterion loosens or replaces it,
// and is refused naming the field it changed.
func TestCreateRejectsALoosenedTemplateCriterion(t *testing.T) {
	tmpl := deliveryTemplate()
	tested, coverage, latency := tmpl.SuccessCriteria[0], tmpl.SuccessCriteria[1], tmpl.SuccessCriteria[2]

	lowerCoverage := coverage
	lowerCoverage.Target = target(70)
	higherLatency := latency
	higherLatency.Target = target(300)
	optionalTested := tested
	optionalTested.Required = required(false)
	judgedTested := tested
	judgedTested.Kind = coreobjective.KindJudged
	judgedTested.Verifier = ""
	judgedTested.Evidence = "the CI log"
	otherVerifier := coverage
	otherVerifier.Verifier = latencyVerifier
	otherVerifier.Metric = "p95_latency_ms"
	otherVerifier.Unit = "ms"
	otherMetric := latency
	otherMetric.Metric = "coverage"
	otherUnit := coverage
	otherUnit.Unit = "ratio"

	for name, tc := range map[string]struct {
		loosened coreobjective.Criterion
		field    string
	}{
		"lower target on >=":   {lowerCoverage, "success_criteria[1].target"},
		"higher target on <=":  {higherLatency, "success_criteria[1].target"},
		"required to optional": {optionalTested, "success_criteria[1].required"},
		"different kind":       {judgedTested, "success_criteria[1].kind"},
		"different verifier":   {otherVerifier, "success_criteria[1].verifier"},
		"different metric":     {otherMetric, "success_criteria[1].metric"},
		"different unit":       {otherUnit, "success_criteria[1].unit"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			// A valid new criterion first, so the index names the request's
			// own position and not the template's.
			assertRejected(t, svc, CreateRequest{
				Title: "ship it", Domain: "software", TemplateID: "tmpl.delivery",
				SuccessCriteria: []coreobjective.Criterion{
					{ID: "reviewed", Description: "an open PR with tests", Kind: coreobjective.KindJudged, Evidence: "the PR"},
					tc.loosened,
				},
			}, tc.field)
		})
	}
}

// Create refuses a criterion it can already tell will never settle, or will be
// settled by somebody it must not be, and names the field.
func TestCreateRejectsAnInvalidCriterion(t *testing.T) {
	discovered := capability.MCPCapabilityID("github", "merge_check")
	measured := func(edit func(*coreobjective.Criterion)) coreobjective.Criterion {
		c := coreobjective.Criterion{
			ID: "core-coverage", Description: "core stays covered", Kind: coreobjective.KindMeasured,
			Verifier: coverageVerifier, Metric: "coverage", Op: coreobjective.OpGreaterEqual, Target: target(90),
			Unit: "percent",
		}
		edit(&c)
		return c
	}

	for name, tc := range map[string]struct {
		criterion coreobjective.Criterion
		field     string
	}{
		"unknown verifier": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: "software.verify.nope"},
			"success_criteria[1].verifier",
		},
		// Reserved even though nothing is registered under it: the namespace
		// alone is refused (ADR 022).
		"reserved verifier": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: "mcp.ghost.check"},
			"success_criteria[1].verifier",
		},
		// Registered, by discovery from a server somebody else runs.
		"discovered verifier": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: discovered},
			"success_criteria[1].verifier",
		},
		"undeclared metric": {
			measured(func(c *coreobjective.Criterion) { c.Metric = "branch_coverage" }),
			"success_criteria[1].metric",
		},
		"unit mismatch": {
			measured(func(c *coreobjective.Criterion) { c.Unit = "ratio" }),
			"success_criteria[1].unit",
		},
		"unknown parameter": {
			measured(func(c *coreobjective.Criterion) { c.Params = map[string]any{"modules": "core"} }),
			"success_criteria[1].params.modules",
		},
		"op without a target": {
			measured(func(c *coreobjective.Criterion) { c.Target = nil }),
			"success_criteria[1].target",
		},
		"judged without evidence": {
			coreobjective.Criterion{ID: "x", Description: "an open PR with tests", Kind: coreobjective.KindJudged},
			"success_criteria[1].evidence",
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
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

// The step 1 rule holds for request criteria too, through the same error, and
// names the list as the field.
func TestCreateRejectsRequestCriteriaWithNoneRequired(t *testing.T) {
	svc := newCriteriaService(t)
	req := CreateRequest{
		Title: "plain", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{
			{ID: "reviewed", Description: "an open PR with tests", Kind: coreobjective.KindJudged, Evidence: "the PR", Required: required(false)},
			{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier, Required: required(false)},
		},
	}
	assertRejected(t, svc, req, "success_criteria")
	if _, err := svc.Create(context.Background(), req); !errors.Is(err, ErrNoRequiredCriterion) {
		t.Errorf("err = %v, want it to wrap ErrNoRequiredCriterion", err)
	}
}
