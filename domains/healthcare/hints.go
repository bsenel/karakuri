package healthcare

import "github.com/bsenel/karakuri/internal/core/domain"

// healthcarePlannerHints guide the loop's action ordering. They are advisory
// — agents may override — but they encode the standing safety norms of the
// domain: observe before act, escalate every treatment, run clinical_review
// last before declaring an objective complete.
func healthcarePlannerHints() []domain.PlannerHint {
	return []domain.PlannerHint{
		{
			Condition: "capability.id startswith 'healthcare.act'",
			Guidance:  "Always observe vitals + medical history + symptoms before executing any healthcare.act capability",
			Priority:  10,
		},
		{
			Condition: "capability.id == 'healthcare.act.recommend_treatment'",
			Guidance:  "recommend_treatment is high-stakes — emit a checkpoint for human approval before recording the plan",
			Priority:  10,
		},
		{
			Condition: "objective.template == 'healthcare.objective.diagnosis_support'",
			Guidance:  "Run healthcare.verify.clinical_review as the final step before marking the objective complete",
			Priority:  9,
		},
		{
			Condition: "capability.id == 'healthcare.act.write_clinical_note'",
			Guidance:  "Always produce a SOAP-format note (subjective/objective/assessment/plan) — leaving any section blank weakens the audit trail",
			Priority:  6,
		},
	}
}
