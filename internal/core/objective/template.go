package objective

import "github.com/bsenel/karakuri/internal/core/agent"

type Template struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Domain      string    `json:"domain"`
	Risk        RiskClass `json:"risk"`
	// Priority is the priority an objective built from this template takes
	// when its request sets none; higher is more urgent. It orders the
	// standing streams when they share the quota (Phase 40 step 9).
	Priority        int                `json:"priority,omitempty"`
	SuccessCriteria []Criterion        `json:"success_criteria,omitempty"`
	Constraints     []Constraint       `json:"constraints,omitempty"`
	SuggestedAgents []agent.Definition `json:"suggested_agents,omitempty"`
	// ActionLimits bound what an action of an objective built from this
	// template may hold. The supervisor adds them to the agent definition's
	// own limits, so a template narrows an agent's authority and never widens
	// it (Phase 40 step 2).
	ActionLimits []agent.ActionLimit `json:"action_limits,omitempty"`
	// BlockerRemedies maps a blocker kind to the text the pack declares as its
	// remedy; the stream's next pass is given the one for its blocker (Phase
	// 40 step 14b). A kind with no entry has no declared remedy.
	BlockerRemedies map[string]string `json:"blocker_remedies,omitempty"`
}
