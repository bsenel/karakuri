package healthcare

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

// healthcareAgentDefinitions returns three agents whose AuthorityBounds are
// deliberately strict — healthcare is a high-stakes domain and every act
// that touches a treatment plan must escalate to a human before firing.
//
//   - triage    — observation + risk scoring only; MaxAutonomousActions = 0
//                 so the agent never executes act.* capabilities on its own.
//   - clinician — full reasoning + low-risk acts (write_clinical_note,
//                 order_test for routine indications); recommend_treatment is
//                 always in RequiresApprovalFor and the confidence threshold
//                 is high enough that uncertain plans escalate too.
//   - auditor   — verify-only; runs guideline_adherence + clinical_review and
//                 is permitted zero autonomous acts.
//
// Together these three roles cover the diagnosis_support and guideline_check
// objective templates without ever bypassing the checkpoint gate.
func healthcareAgentDefinitions() []agent.Definition {
	mem := agent.MemoryConfig{
		WorkingWindowSize: 24,
		EpisodicRetention: 180 * 24 * time.Hour, // 6 months
		SemanticEnabled:   true,
		ProceduralEnabled: true,
	}

	caps := func(ids ...string) []capability.CapabilityID {
		out := make([]capability.CapabilityID, len(ids))
		for i, id := range ids {
			out[i] = capability.CapabilityID(id)
		}
		return out
	}

	return []agent.Definition{
		{
			ID:     "healthcare.agent.triage",
			Name:   "Triage Nurse",
			Domain: "healthcare",
			Capabilities: caps(
				"healthcare.observe.vital_signs",
				"healthcare.observe.symptoms",
				"healthcare.observe.medical_history",
				"healthcare.reason.risk_assessment",
				"healthcare.decide.triage_priority",
			),
			Memory:            mem,
			ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 0, // pure read + reason; no acts
				ConfidenceThreshold:  0.85,
				CanDelegate:          false,
				CanModifyObjective:   false,
			},
			LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.3},
		},
		{
			ID:     "healthcare.agent.clinician",
			Name:   "Attending Clinician",
			Domain: "healthcare",
			Capabilities: caps(
				"healthcare.observe.vital_signs",
				"healthcare.observe.lab_results",
				"healthcare.observe.medical_history",
				"healthcare.observe.symptoms",
				"healthcare.reason.differential_diagnosis",
				"healthcare.reason.risk_assessment",
				"healthcare.decide.triage_priority",
				"healthcare.act.order_test",
				"healthcare.act.recommend_treatment",
				"healthcare.act.write_clinical_note",
				"healthcare.learn.case_summary",
			),
			Memory:            mem,
			ReasoningStrategy: agent.ReasoningReflexion,
			Authority: agent.AuthorityBounds{
				// Low-risk acts only (note + routine order); even those are
				// limited so a runaway loop can't flood orders.
				MaxAutonomousActions: 3,
				ConfidenceThreshold:  0.85,
				RequiresApprovalFor: caps(
					"healthcare.act.recommend_treatment",
				),
				CanDelegate:        false,
				CanModifyObjective: false,
			},
			LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.3},
		},
		{
			ID:     "healthcare.agent.auditor",
			Name:   "Clinical Auditor",
			Domain: "healthcare",
			Capabilities: caps(
				"healthcare.observe.medical_history",
				"healthcare.observe.lab_results",
				"healthcare.verify.guideline_adherence",
				"healthcare.verify.clinical_review",
			),
			Memory:            mem,
			ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 0,
				ConfidenceThreshold:  0.90, // stricter — auditor catches edge cases
				CanDelegate:          false,
				CanModifyObjective:   false,
			},
			LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.2},
		},
	}
}
