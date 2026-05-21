package healthcare

import (
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// healthcareObjectiveTemplates returns the two flagship templates:
//
//   - diagnosis_support — observe vitals/labs/history/symptoms, generate a
//     differential, propose a treatment plan, then gate it on both
//     guideline_adherence AND clinical_review. The plan-then-verify sequence
//     mirrors how high-stakes clinical decisions actually unfold.
//
//   - guideline_check   — narrower scope: read a patient's active plan and
//     verify it still matches the current published guideline (e.g. when
//     the guideline version is updated). Produces the same clinical_review
//     artifact for audit.
//
// Constraints declare hard ordering: vitals + history must be observed
// before any reason capability; recommend_treatment requires explicit
// approval; clinical_review must succeed before the objective is marked
// complete.
func healthcareObjectiveTemplates() []objective.Template {
	crit := func(id, desc, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID:          id,
			Description: desc,
			Verifier:    capability.CapabilityID(verifier),
			Weight:      weight,
		}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}

	return []objective.Template{
		{
			ID:          "healthcare.objective.diagnosis_support",
			Title:       "Diagnosis Support",
			Domain:      "healthcare",
			Description: "Synthesise vitals, labs, history, and symptoms into a differential, propose a treatment plan, and verify against guideline + senior review",
			SuccessCriteria: []objective.Criterion{
				crit("differential-quality",
					"Differential diagnosis produced with at least 3 ranked candidates and supporting evidence",
					"healthcare.reason.differential_diagnosis", 0.25),
				crit("guideline-adherence",
					"Proposed plan adheres to the applicable clinical guideline",
					"healthcare.verify.guideline_adherence", 0.35),
				crit("clinical-review",
					"Senior clinician approves the plan via clinical_review",
					"healthcare.verify.clinical_review", 0.40),
			},
			Constraints: []objective.Constraint{
				hard("observe-first",
					"Vitals AND medical history AND symptoms must be observed before any reason capability",
					"vitals_observed && history_observed && symptoms_observed"),
				hard("treatment-requires-approval",
					"recommend_treatment must escalate to a human checkpoint before the plan is recorded",
					"recommend_treatment_approved"),
				hard("review-before-complete",
					"clinical_review must succeed before the objective can be marked complete",
					"clinical_review_passed"),
			},
		},
		{
			ID:          "healthcare.objective.guideline_check",
			Title:       "Guideline Check",
			Domain:      "healthcare",
			Description: "Audit a patient's active care plan against the current published clinical guideline and surface deviations",
			SuccessCriteria: []objective.Criterion{
				crit("history-loaded",
					"Patient history + active plan loaded from EHR",
					"healthcare.observe.medical_history", 0.20),
				crit("guideline-adherence",
					"Active plan checked against the current guideline version",
					"healthcare.verify.guideline_adherence", 0.50),
				crit("review-recorded",
					"Audit produces a clinical_review artifact with deviations + recommendations",
					"healthcare.verify.clinical_review", 0.30),
			},
			Constraints: []objective.Constraint{
				hard("history-first",
					"Patient history must be observed before guideline checking",
					"history_observed"),
			},
		},
	}
}
