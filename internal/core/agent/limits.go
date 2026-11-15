package agent

import (
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// ActionLimit bounds the values one parameter of one capability's actions may
// hold. The core compares strings with patterns and knows nothing of what the
// parameter means; only the pack that declares the limit knows that one
// parameter is a tool allow-list and another a branch name (Phase 40 step 1).
type ActionLimit struct {
	// Capability is the capability whose actions the limit applies to.
	Capability capability.CapabilityID
	// Parameter is the name of the action parameter the limit reads.
	Parameter string
	// Patterns are path.Match patterns. A string value, or every element of a
	// list value, must match at least one of them.
	Patterns []string
}

// ActionLimitViolation is one planned action whose parameter is outside a
// declared limit.
type ActionLimitViolation struct {
	// Action is the capability of the offending action.
	Action capability.CapabilityID
	// Parameter is the parameter that broke the limit.
	Parameter string
	// Value is the offending value, or empty when the parameter is missing.
	Value string
	// Limit is the limit that was broken.
	Limit ActionLimit
}

// CheckActionLimits reports every planned action whose parameters fall outside
// the bounds' action limits.
//
// Phase 40 step 1 scaffolding: the check is not written yet, so this reports
// no violation and the tests beside it are red.
func (b AuthorityBounds) CheckActionLimits(actions []environment.Action) []ActionLimitViolation {
	return nil
}

// DecideActions is Decide over the planned actions themselves, so the bounds
// can read each action's parameters as well as its capability.
//
// Phase 40 step 1 scaffolding: it does not yet check the action limits.
func (b AuthorityBounds) DecideActions(confidence, threshold float64, actions []environment.Action, writes []string, ev Evidence) Verdict {
	planned := make([]capability.CapabilityID, 0, len(actions))
	for _, a := range actions {
		planned = append(planned, a.CapabilityID)
	}
	return b.Decide(confidence, threshold, planned, writes, ev)
}
