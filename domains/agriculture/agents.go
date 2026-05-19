package agriculture

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

func agricultureAgentDefinitions() []agent.Definition {
	mem := agent.MemoryConfig{
		WorkingWindowSize: 16,
		EpisodicRetention: 90 * 24 * time.Hour,
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
			ID:     "agriculture.agent.field_manager",
			Name:   "Field Manager",
			Domain: "agriculture",
			Capabilities: caps(
				"agriculture.observe.soil_conditions",
				"agriculture.observe.crop_health",
				"agriculture.reason.irrigation_plan",
				"agriculture.act.irrigate",
				"agriculture.act.apply_treatment",
			),
			Memory:            mem,
			ReasoningStrategy: agent.ReasoningReAct,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 10,
				ConfidenceThreshold:  0.75,
				RequiresApprovalFor: caps(
					"agriculture.act.apply_treatment",
				),
				CanDelegate:        false,
				CanModifyObjective: false,
			},
			LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.4},
		},
		{
			ID:     "agriculture.agent.analyst",
			Name:   "Crop Analyst",
			Domain: "agriculture",
			Capabilities: caps(
				"agriculture.observe.soil_conditions",
				"agriculture.observe.weather",
				"agriculture.observe.crop_health",
				"agriculture.reason.yield_forecast",
				"agriculture.verify.yield_target",
			),
			Memory:            mem,
			ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 0,
				ConfidenceThreshold:  0.80,
				CanDelegate:          false,
				CanModifyObjective:   false,
			},
			LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.5},
		},
	}
}
