package karakuri

import (
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

func karakuriAgentDefinitions() []agent.Definition {
	return []agent.Definition{
		{
			ID:     "karakuri-maintainer",
			Name:   "Karakuri Maintainer",
			Domain: "karakuri",
			Capabilities: []capability.CapabilityID{
				CapAnalyseUsage,
				CapProposeRoadmap,
				CapDraftADR,
			},
			// Reflexion, because this agent's output is a proposal somebody
			// will read as evidence-backed, and a critique-and-revise pass is
			// the cheapest available defence against a confident paragraph
			// with nothing behind it.
			ReasoningStrategy: agent.ReasoningReflexion,
			Authority: agent.AuthorityBounds{
				// Zero, and deliberately not a small number.
				//
				// This is the agent that reasons about what Karakuri should
				// become. Every action it plans escalates, whatever the
				// standing objective's autonomy level says — the ceiling on
				// the objective bounds how far it may be promoted, and this
				// bounds what promotion can ever mean here. A self-improving
				// system that could act on its own conclusions about itself
				// unsupervised is the one shape of this feature nobody asked
				// for.
				MaxAutonomousActions: 0,
				RequiresApprovalFor: []capability.CapabilityID{
					CapProposeRoadmap,
					CapDraftADR,
				},
				CanDelegate: false,
				// The agent that decides what Karakuri should do must not be
				// able to edit what it was asked to do.
				CanModifyObjective:  false,
				ConfidenceThreshold: 1.0,
			},
			Memory: agent.MemoryConfig{
				SemanticEnabled:   true,
				ProceduralEnabled: true,
			},
		},
		{
			ID:     "karakuri-analyst",
			Name:   "Karakuri Analyst",
			Domain: "karakuri",
			// Reading only. Separate from the maintainer so an operator can
			// run "tell me what is limiting this deployment" as a standing
			// objective at sense autonomy without ever putting an agent that
			// drafts changes into the rotation.
			Capabilities:      []capability.CapabilityID{CapAnalyseUsage},
			ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 1,
				CanDelegate:          false,
				CanModifyObjective:   false,
				ConfidenceThreshold:  0.7,
			},
			Memory: agent.MemoryConfig{SemanticEnabled: true},
		},
	}
}
