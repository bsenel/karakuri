package software

import (
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// docsTemplateID is the docs-only one-shot Phase 41 step 3 adds.
const docsTemplateID = "software.objective.docs"

// Phase 41 step 3, the template table: each template's required criteria name
// a deterministic verifier, and every judged criterion is kept as progress,
// required: false with a weight, so a model's reading can no longer fail a
// pass that did the work.
var deterministicCriteriaCases = []struct {
	id       string
	required []string
}{
	{marketDiscoveryTemplateID, []string{CapStreamPR, CapPathsDeclared, CapCIGreen}},
	{engineeringBacklogTemplateID, []string{CapStreamPR, CapPathsDeclared, CapCIGreen}},
	{uxImprovementTemplateID, []string{CapStreamPR, CapPathsDeclared, CapCIGreen}},
	{roadmapDeliveryTemplateID, []string{CapStreamPR, CapCIGreen, "software.verify.run_tests"}},
	{"software.objective.delivery", []string{"software.verify.run_tests", "software.verify.lint"}},
	{docsTemplateID, []string{CapStreamPR, CapPathsDeclared, CapCIGreen}},
}

func TestTemplatesRequireDeterministicVerifiers(t *testing.T) {
	for _, tc := range deterministicCriteriaCases {
		t.Run(tc.id, func(t *testing.T) {
			tpl := streamTemplate(t, tc.id)
			var got []string
			for _, c := range tpl.SuccessCriteria {
				if c.IsRequired() {
					got = append(got, string(c.Verifier))
				}
			}
			for _, want := range tc.required {
				if !slices.Contains(got, want) {
					t.Errorf("required verifiers = %v, missing %s", got, want)
				}
			}
		})
	}
}

func TestJudgedTemplateCriteriaAreProgressNotGates(t *testing.T) {
	for _, tc := range deterministicCriteriaCases {
		t.Run(tc.id, func(t *testing.T) {
			tpl := streamTemplate(t, tc.id)
			for _, c := range tpl.SuccessCriteria {
				if c.Verifier != "" {
					continue
				}
				if c.IsRequired() {
					t.Errorf("judged criterion %s is required; a model's reading must not fail a pass", c.ID)
				}
				if c.Weight <= 0 {
					t.Errorf("judged criterion %s has weight %v; it is kept to score progress", c.ID, c.Weight)
				}
			}
		})
	}
}

// Each paths_declared criterion carries the globs the template's row names,
// so a criterion that ignored them would pass any diff.
func TestPathsDeclaredCriteriaCarryTheirGlobs(t *testing.T) {
	for _, tc := range []struct {
		id    string
		globs []string
	}{
		{marketDiscoveryTemplateID, []string{"docs/research/**"}},
		{engineeringBacklogTemplateID, []string{"docs/roadmap.md"}},
		{uxImprovementTemplateID, []string{"cli/**", "web/**"}},
		{docsTemplateID, []string{"docs/**"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			tpl := streamTemplate(t, tc.id)
			var found bool
			for _, c := range tpl.SuccessCriteria {
				if string(c.Verifier) != CapPathsDeclared {
					continue
				}
				found = true
				got := pathGlobsParam(map[string]any{"paths": c.Threshold})
				if !slices.Equal(got, tc.globs) {
					t.Errorf("paths_declared globs = %v, want %v", got, tc.globs)
				}
			}
			if !found {
				t.Error("no paths_declared criterion")
			}
		})
	}
}

// Replays #174's state: the UX pass opened a pull request with tests and its
// checks were green. Under the new criteria every required one is settled met
// by its verifier, with no judge.
func TestReplayOfPR174MeetsTheUXTemplate(t *testing.T) {
	tpl := streamTemplate(t, uxImprovementTemplateID)
	settled := map[string]objective.CriterionState{
		CapStreamPR:      objective.CriterionMet,
		CapPathsDeclared: objective.CriterionMet,
		CapCIGreen:       objective.CriterionMet,
	}
	criteria := slices.Clone(tpl.SuccessCriteria)
	for i, c := range criteria {
		if c.Verifier == "" {
			// The judge is not asked; a judged criterion is left unsettled.
			continue
		}
		criteria[i].State = settled[string(c.Verifier)]
	}
	if !objective.AllRequiredMet(criteria) {
		t.Errorf("#174's state does not meet the UX template: %+v", criteria)
	}
}
