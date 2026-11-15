package objective

import "github.com/bsenel/karakuri/internal/core/agent"

type Template struct {
	ID              string             `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description,omitempty"`
	Domain          string             `json:"domain"`
	Risk            RiskClass          `json:"risk"`
	SuccessCriteria []Criterion        `json:"success_criteria,omitempty"`
	Constraints     []Constraint       `json:"constraints,omitempty"`
	SuggestedAgents []agent.Definition `json:"suggested_agents,omitempty"`
	// ActionLimits bound what an action of an objective built from this
	// template may hold. The supervisor adds them to the agent definition's
	// own limits, so a template narrows an agent's authority and never widens
	// it (Phase 40 step 2).
	ActionLimits []agent.ActionLimit `json:"action_limits,omitempty"`
}
