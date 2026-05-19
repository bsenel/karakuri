package software

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

func softwareAgentDefinitions() []agent.Definition {
	mem := agent.MemoryConfig{
		WorkingWindowSize: 20,
		EpisodicRetention: 30 * 24 * time.Hour,
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

	authority := func(maxAuto int, threshold float64, requiresApproval ...string) agent.AuthorityBounds {
		req := make([]capability.CapabilityID, len(requiresApproval))
		for i, id := range requiresApproval {
			req[i] = capability.CapabilityID(id)
		}
		return agent.AuthorityBounds{
			MaxAutonomousActions: maxAuto,
			ConfidenceThreshold:  threshold,
			RequiresApprovalFor:  req,
		}
	}

	return []agent.Definition{
		{
			ID: "software.agent.strategist", Name: "Strategist", Domain: "software",
			Capabilities: caps("reason.synthesize", "reason.plan", "software.reason.research"),
			Memory: mem, ReasoningStrategy: agent.ReasoningReflexion,
			Authority: authority(0, 0.9, "software.act.write_code"),
			LLMHints:  capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.7},
		},
		{
			ID: "software.agent.architect", Name: "Architect", Domain: "software",
			Capabilities: caps("software.reason.architecture_review", "software.act.write_design_doc", "reason.evaluate"),
			Memory: mem, ReasoningStrategy: agent.ReasoningTreeOfThought,
			Authority: authority(5, 0.8),
			LLMHints:  capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.8},
		},
		{
			ID: "software.agent.researcher", Name: "Researcher", Domain: "software",
			Capabilities: caps("software.reason.research", "observe.fetch_signal", "reason.synthesize"),
			Memory: mem, ReasoningStrategy: agent.ReasoningReAct,
			Authority: authority(10, 0.7),
			LLMHints:  capability.LLMHints{PreferredProvider: "gemini", FallbackProvider: "claude", TemperatureMax: 0.6},
		},
		{
			ID: "software.agent.implementer", Name: "Implementer", Domain: "software",
			Capabilities: caps("software.act.write_code", "software.act.write_test", "software.verify.run_tests", "software.verify.lint"),
			Memory: mem, ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: authority(20, 0.75),
			LLMHints:  capability.LLMHints{PreferredProvider: "cursor", FallbackProvider: "claude", TemperatureMax: 0.3},
		},
		{
			ID: "software.agent.reviewer", Name: "Reviewer", Domain: "software",
			Capabilities: caps("software.verify.review", "software.verify.tech_lead_review", "reason.evaluate"),
			Memory: mem, ReasoningStrategy: agent.ReasoningReflexion,
			Authority: authority(0, 0.85),
			LLMHints:  capability.LLMHints{PreferredProvider: "copilot", FallbackProvider: "claude", TemperatureMax: 0.5},
		},
		{
			ID: "software.agent.sre", Name: "SRE", Domain: "software",
			Capabilities: caps("software.observe.fetch_logs", "software.observe.fetch_metrics", "software.act.write_code", "software.verify.run_tests"),
			Memory: mem, ReasoningStrategy: agent.ReasoningReAct,
			Authority: authority(10, 0.8, "software.act.create_pr"),
			LLMHints:  capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.4},
		},
		{
			ID: "software.agent.watcher", Name: "Watcher", Domain: "software",
			Capabilities: caps(
				"software.observe.fetch_commits", "software.observe.fetch_prs",
				"software.observe.fetch_logs", "software.observe.fetch_metrics",
				"software.observe.read_codebase",
			),
			Memory: mem, ReasoningStrategy: agent.ReasoningReAct,
			Authority: authority(0, 0.95),
			LLMHints:  capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.3},
		},
	}
}
