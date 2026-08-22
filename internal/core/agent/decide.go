package agent

import (
	"fmt"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// Verdict is what a set of authority bounds permits for one plan.
type Verdict struct {
	// Escalate says the plan needs a human before any of it runs.
	Escalate bool
	// Reason names which bound escalated, in the words an operator reads on
	// the checkpoint. Empty when Escalate is false.
	Reason string
	// Allowed is how many of the plan's actions survive the autonomous-action
	// cap. It is not zero when Escalate is true: an escalating plan is shown
	// to the reviewer intact, and an approval falls straight through to the
	// act step, so an emptied plan would silently discard approved work.
	Allowed int
}

// Decide applies the bounds to a plan and reports what they permit.
//
// It is the whole policy: every question about what an agent may do without
// asking is answered here, and stepDecide's job is to carry out the answer —
// write the audit row, raise the checkpoint, trim the plan. Splitting it out
// is what makes a declared bound testable by running it rather than by reading
// the field back, which is the distinction Phase 24 exists for. Four packs
// once wrote MaxAutonomousActions: 0 to mean "plans but never acts", one of
// them saying so in a comment on the line, while the loop read zero as "no cap
// at all" — and every test asserted the field *was* zero, none asserted what
// zero *did*.
//
// threshold is the effective confidence threshold, which an operator may lower
// for one iteration but never raise; callers pass the result of that
// adjustment rather than the declared value.
//
// The order of the checks is load-bearing. A more specific reason wins: a plan
// that is both under-confident and unbounded reports the confidence, because
// that is the one the operator can act on.
func (b AuthorityBounds) Decide(confidence, threshold float64, plannedCapabilities []capability.CapabilityID) Verdict {
	v := Verdict{Allowed: len(plannedCapabilities)}

	// 1. Confidence. A threshold of zero opts out rather than escalating
	// everything — an agent that declared no threshold has no opinion.
	if threshold > 0 && confidence < threshold {
		v.Escalate = true
		v.Reason = fmt.Sprintf("confidence %.2f below threshold %.2f", confidence, threshold)
	}

	// 2. Capabilities the bounds name as needing approval.
	if !v.Escalate && len(b.RequiresApprovalFor) > 0 {
		approvalSet := make(map[capability.CapabilityID]struct{}, len(b.RequiresApprovalFor))
		for _, c := range b.RequiresApprovalFor {
			approvalSet[c] = struct{}{}
		}
		for _, c := range plannedCapabilities {
			if _, requires := approvalSet[c]; requires {
				v.Escalate = true
				v.Reason = fmt.Sprintf("action %q requires approval", c)
				break
			}
		}
	}

	// 3. A cap of zero is the "draft and ask" bound: no autonomous actions at
	// all. It escalates rather than trimming, for the reason on Verdict.Allowed.
	if !v.Escalate && b.MaxAutonomousActions == 0 && len(plannedCapabilities) > 0 {
		v.Escalate = true
		v.Reason = "agent is bounded to no autonomous actions"
	}

	// 4. Trim to the cap. UnlimitedActions (negative) and zero both opt out of
	// trimming — zero because it escalated above.
	if b.MaxAutonomousActions > 0 && v.Allowed > b.MaxAutonomousActions {
		v.Allowed = b.MaxAutonomousActions
	}

	return v
}
