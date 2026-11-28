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
	// BlockerAttempts is how many failed diagnose-and-fix passes on the same
	// blocker the stream makes before it stops and hands the blocker to the
	// backlog (Phase 40 step 14c); 0 means DefaultBlockerAttempts.
	BlockerAttempts int `json:"blocker_attempts,omitempty"`
	// BlockerRecurrenceWindow is the Go duration inside which a blocker kind
	// cleared for the second time is handed to the backlog (Phase 40 step
	// 14c); empty means DefaultBlockerRecurrenceWindow.
	BlockerRecurrenceWindow string `json:"blocker_recurrence_window,omitempty"`
}

// The defaults of a template that declares no blocker limits (Phase 40 step
// 14c): three attempts, as a first guess, and a week.
const (
	DefaultBlockerAttempts         = 3
	DefaultBlockerRecurrenceWindow = "168h"
)
