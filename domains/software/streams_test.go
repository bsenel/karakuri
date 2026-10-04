package software

import (
	"math"
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// streamCases is what each standing stream template is pinned to: how its
// author regards a mistake, and the agent whose bounds it runs under.
var streamCases = []struct {
	id    string
	risk  objective.RiskClass
	agent string
}{
	{marketDiscoveryTemplateID, objective.RiskRoutine, "software.agent.strategist"},
	{engineeringBacklogTemplateID, objective.RiskRoutine, "software.agent.maintainer"},
	{uxImprovementTemplateID, objective.RiskConsequential, "software.agent.implementer"},
	{roadmapDeliveryTemplateID, objective.RiskConsequential, "software.agent.implementer"},
}

// streamTemplate goes through the pack rather than streamTemplates(): a
// template the pack does not return is one nobody can instantiate.
func streamTemplate(t *testing.T, id string) objective.Template {
	t.Helper()
	for _, tpl := range New().ObjectiveTemplates() {
		if tpl.ID == id {
			return tpl
		}
	}
	t.Fatalf("the software pack declares no %s template", id)
	return objective.Template{}
}

func TestStreamTemplatesAreDeclaredByThePack(t *testing.T) {
	declared := map[string]int{}
	for _, tpl := range New().ObjectiveTemplates() {
		declared[tpl.ID]++
	}
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			if n := declared[tc.id]; n != 1 {
				t.Errorf("the pack returns %s %d times, want 1", tc.id, n)
			}
		})
	}
}

// An unclassified template is one whose author has not said how they regard
// it, which is not an answer a standing objective can run on.
func TestStreamTemplatesAreRiskClassified(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			tpl := streamTemplate(t, tc.id)
			if !tpl.Risk.Valid() {
				t.Errorf("Risk = %q, which is not a risk class", tpl.Risk)
			}
			if tpl.Risk == objective.RiskUnclassified {
				t.Error("the template is unclassified")
			}
			if tpl.Risk != tc.risk {
				t.Errorf("Risk = %q, want %q", tpl.Risk, tc.risk)
			}
		})
	}
}

// Without a named agent the objective inherits whichever agent this pack
// declares first.
func TestStreamTemplatesNameTheirAgent(t *testing.T) {
	declared := map[string]bool{}
	for _, a := range New().AgentDefinitions() {
		declared[string(a.ID)] = true
	}
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			tpl := streamTemplate(t, tc.id)
			if len(tpl.SuggestedAgents) == 0 {
				t.Fatalf("%s names no agent; it would run under whichever this pack declares first", tc.id)
			}
			for _, a := range tpl.SuggestedAgents {
				if !declared[string(a.ID)] {
					t.Errorf("%s names agent %q, which this pack does not declare", tc.id, a.ID)
				}
			}
			if got := string(tpl.SuggestedAgents[0].ID); got != tc.agent {
				t.Errorf("%s names %q, want %q", tc.id, got, tc.agent)
			}
		})
	}
}

// Every stream criterion is judged: no capability's success answers it, so
// naming a verifier would claim a settlement that never ran.
func TestStreamCriteriaAreJudged(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			for _, crit := range streamTemplate(t, tc.id).SuccessCriteria {
				if crit.Verifier != "" {
					t.Errorf("criterion %q is verified by %q, want it judged", crit.ID, crit.Verifier)
				}
			}
		})
	}
}

func TestStreamCriteriaWeightsSumToOne(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			criteria := streamTemplate(t, tc.id).SuccessCriteria
			if len(criteria) != 2 {
				t.Fatalf("%d criteria, want 2", len(criteria))
			}
			var sum float64
			for _, crit := range criteria {
				sum += crit.Weight
			}
			if math.Abs(sum-1.0) > 1e-9 {
				t.Errorf("criterion weights sum to %v, want 1.0", sum)
			}
		})
	}
}

// A stream keeps running, so what it must never do is a hard constraint
// rather than something each iteration is trusted to remember.
func TestStreamTemplatesForbidMerging(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			for _, c := range streamTemplate(t, tc.id).Constraints {
				if c.ID != "no-merge" {
					continue
				}
				if !c.Hard {
					t.Error("no-merge is not a hard constraint")
				}
				if c.Expression != "no_merge" {
					t.Errorf("no-merge expression = %q, want %q", c.Expression, "no_merge")
				}
				return
			}
			t.Error("the template carries no no-merge constraint")
		})
	}
}
