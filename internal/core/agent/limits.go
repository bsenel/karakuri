package agent

import (
	"fmt"
	"path"
	"strings"

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

// admits reports whether value matches one of the limit's patterns. A pattern
// path.Match cannot parse matches nothing: a typo in a declared limit must
// narrow the bound, never open it.
func (l ActionLimit) admits(value string) bool {
	for _, p := range l.Patterns {
		if ok, err := path.Match(p, value); err == nil && ok {
			return true
		}
	}
	return false
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
	// missing distinguishes an absent parameter from one holding "".
	missing bool
}

// reason is the violation in the words an operator reads on the checkpoint.
func (v ActionLimitViolation) reason() string {
	patterns := strings.Join(v.Limit.Patterns, ", ")
	if v.missing {
		return fmt.Sprintf("action %q has no parameter %q, which its limit [%s] requires", v.Action, v.Parameter, patterns)
	}
	return fmt.Sprintf("action %q parameter %q holds %q, outside its limit [%s]", v.Action, v.Parameter, v.Value, patterns)
}

// CheckActionLimits reports every planned action whose parameters fall outside
// the bounds' action limits.
//
// Every limit on an action's capability must hold, so two limits on one
// parameter intersect: that is what lets a narrower declaration be added
// beside a wider one without widening it. A parameter the action leaves out is
// a violation, because bounds fail toward refusing: an action that does not
// say what it holds has not shown it is inside the limit. A list value is
// inside only when every element is, and an element that is not a string is
// outside, since there is nothing to match it against.
func (b AuthorityBounds) CheckActionLimits(actions []environment.Action) []ActionLimitViolation {
	var out []ActionLimitViolation
	for _, a := range actions {
		for _, l := range b.ActionLimits {
			if l.Capability != a.CapabilityID {
				continue
			}
			raw, ok := a.Params[l.Parameter]
			if !ok {
				out = append(out, ActionLimitViolation{Action: a.CapabilityID, Parameter: l.Parameter, Limit: l, missing: true})
				continue
			}
			for _, value := range limitValues(raw) {
				s, isString := value.(string)
				if isString && l.admits(s) {
					continue
				}
				out = append(out, ActionLimitViolation{Action: a.CapabilityID, Parameter: l.Parameter, Value: fmt.Sprint(value), Limit: l})
			}
		}
	}
	return out
}

// limitValues spreads a parameter into the values a limit matches one by one:
// the elements of a list, or the value itself. A plan decoded from JSON carries
// lists as []any; one built in Go may carry []string.
func limitValues(raw any) []any {
	switch v := raw.(type) {
	case []any:
		return v
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	default:
		return []any{raw}
	}
}

// DecideActions is Decide over the planned actions themselves, so the bounds
// can read each action's parameters as well as its capability.
//
// With no action limits declared it is Decide. A limit violation escalates and
// its reason leads, because it is the one bound that says what a specific
// action would do outside its authority; any reason Decide escalated for as
// well follows it, so the reviewer is not shown one bound and left to approve
// past another. Allowed is Decide's: an escalated plan is shown intact.
func (b AuthorityBounds) DecideActions(confidence, threshold float64, actions []environment.Action, writes []string, ev Evidence) Verdict {
	planned := make([]capability.CapabilityID, 0, len(actions))
	for _, a := range actions {
		planned = append(planned, a.CapabilityID)
	}
	v := b.Decide(confidence, threshold, planned, writes, ev)

	violations := b.CheckActionLimits(actions)
	if len(violations) == 0 {
		return v
	}
	reasons := make([]string, 0, len(violations)+1)
	for _, viol := range violations {
		reasons = append(reasons, viol.reason())
	}
	if v.Escalate {
		reasons = append(reasons, v.Reason)
	}
	v.Escalate = true
	v.Reason = strings.Join(reasons, "; ")
	return v
}
