package conformance_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Every template a shipped pack declares says how its author regards it.
//
// Unclassified is a legal value — a third-party pack written before the field
// existed must still load — but it is not an acceptable one for a pack this
// repository ships, and nothing else would notice a new template added
// without one.
func TestShippedTemplatesAreClassified(t *testing.T) {
	templates := 0
	for _, p := range allShippedPacks() {
		for _, tmpl := range p.ObjectiveTemplates() {
			templates++
			if !tmpl.Risk.Valid() {
				t.Errorf("pack %q template %q declares risk %q, which is not a risk class", p.ID(), tmpl.ID, string(tmpl.Risk))
				continue
			}
			if tmpl.Risk == objective.RiskUnclassified {
				t.Errorf("pack %q template %q is unclassified: set Template.Risk", p.ID(), tmpl.ID)
			}
		}
	}
	if templates == 0 {
		t.Fatal("no shipped pack declares a template; this test checked nothing")
	}
}

func TestCheckTemplateRisk(t *testing.T) {
	for _, tc := range []struct {
		name    string
		risk    objective.RiskClass
		passed  bool
		warning bool
		says    string
	}{
		{"classified passes", objective.RiskConsequential, true, false, "consequential"},
		{"unclassified warns without failing", objective.RiskUnclassified, true, true, "unclassified"},
		{"a value outside the set fails", objective.RiskClass("critical"), false, false, "critical"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := conformance.CheckTemplateRisk(packWithRisk("routing.objective.fixture", tc.risk))
			if len(results) != 1 {
				t.Fatalf("expected one result for one template, got %+v", results)
			}
			res := results[0]
			if res.Check != "template_risk" {
				t.Errorf("check = %q, want %q", res.Check, "template_risk")
			}
			if res.Passed != tc.passed {
				t.Errorf("Passed = %v, want %v (%s)", res.Passed, tc.passed, res.Message)
			}
			if res.Warning != tc.warning {
				t.Errorf("Warning = %v, want %v (%s)", res.Warning, tc.warning, res.Message)
			}
			for _, want := range []string{`"routing.objective.fixture"`, `domain "routing"`, tc.says} {
				if !strings.Contains(res.Message, want) {
					t.Errorf("message %q does not name %s", res.Message, want)
				}
			}
		})
	}
}

// One result per template, so a single boot pass names every template that
// needs attention instead of stopping at the first.
func TestCheckTemplateRiskReportsEveryTemplate(t *testing.T) {
	a := packWithRisk("routing.objective.classified", objective.RiskRoutine)
	b := packWithRisk("routing.objective.unclassified", objective.RiskUnclassified)
	c := packWithRisk("routing.objective.typo", objective.RiskClass("HIGH"))

	results := conformance.CheckTemplateRisk(a, b, c)
	if len(results) != 3 {
		t.Fatalf("expected three results, got %+v", results)
	}
	failed, warned := 0, 0
	for _, res := range results {
		if !res.Passed {
			failed++
			if !strings.Contains(res.Message, "routing.objective.typo") {
				t.Errorf("the failure %q does not name the offending template", res.Message)
			}
		}
		if res.Warning {
			warned++
			if !strings.Contains(res.Message, "routing.objective.unclassified") {
				t.Errorf("the warning %q does not name the unclassified template", res.Message)
			}
		}
	}
	if failed != 1 || warned != 1 {
		t.Errorf("expected exactly one failure and one warning, got %d and %d: %+v", failed, warned, results)
	}
}

// packWithRisk returns a pack whose single template declares the given risk.
func packWithRisk(id string, risk objective.RiskClass) domain.Pack {
	p := &templatePack{}
	p.templates = []objective.Template{{
		ID:     id,
		Title:  "fixture",
		Domain: "routing",
		Risk:   risk,
	}}
	return p
}
