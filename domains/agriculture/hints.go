package agriculture

import "github.com/bsenel/karakuri/internal/core/domain"

func agriculturePlannerHints() []domain.PlannerHint {
	return []domain.PlannerHint{
		{
			Condition: "capability.id startswith 'agriculture.act'",
			Guidance:  "Always observe soil conditions and crop health before executing any act capability",
			Priority:  10,
		},
		{
			Condition: "objective.template == 'agriculture.objective.optimize_yield'",
			Guidance:  "Run agriculture.verify.yield_target as the final step before marking the objective complete",
			Priority:  9,
		},
	}
}
