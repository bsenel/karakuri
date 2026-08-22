package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// Evidence is where the material a plan was drafted from came from.
//
// It carries only the sources whose payloads somebody outside this deployment
// wrote — the environments that declared environment.TrustThirdParty on an
// observation or an action result. Everything else is the operator's own
// infrastructure and needs no naming here.
//
// It is deliberately a set of names rather than the material itself. The policy
// does not read prose and does not rank it by how suspicious it looks: it
// answers "was a stranger's writing in front of the planner when it drafted
// this", which is a property that holds against attacks nobody has thought of
// yet, where a classifier is only as good as its last training set.
type Evidence struct {
	// ThirdParty names the environments that contributed such a payload,
	// sorted and deduplicated by WithSource so an escalation reason and the
	// audit row beside it read the same across runs.
	ThirdParty []string
}

// HasThirdParty reports whether a stranger's writing reached the planner.
func (e Evidence) HasThirdParty() bool { return len(e.ThirdParty) > 0 }

// WithSource returns a copy naming one more third-party source, deduplicated
// and sorted so an escalation reason reads the same across runs.
//
// Copy rather than mutate: the loop threads one Evidence value through six
// steps and two of them hand it to a policy that must not be able to widen it.
func (e Evidence) WithSource(envID string) Evidence {
	if envID == "" {
		return e
	}
	for _, existing := range e.ThirdParty {
		if existing == envID {
			return e
		}
	}
	next := make([]string, 0, len(e.ThirdParty)+1)
	next = append(next, e.ThirdParty...)
	next = append(next, envID)
	sort.Strings(next)
	return Evidence{ThirdParty: next}
}

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
// ev is where the plan's material came from. It is a parameter rather than a
// new branch over existing inputs because the bounds saw no observations and no
// per-action provenance at all: everything the policy knew was about the plan,
// and nothing about what the plan was drafted from. Adding the input is the
// change; the branch is the small part.
//
// The order of the checks is load-bearing. A more specific reason wins: a plan
// that is both under-confident and unbounded reports the confidence, because
// that is the one the operator can act on. Provenance goes first for the same
// reason one step further: "a stranger's writing was in front of the planner"
// is a fact about the world, actionable by looking at the named source, where
// confidence is the model's own report on itself.
func (b AuthorityBounds) Decide(confidence, threshold float64, plannedCapabilities []capability.CapabilityID, ev Evidence) Verdict {
	v := Verdict{Allowed: len(plannedCapabilities)}

	// 1. Provenance. A plan drafted while somebody else's writing was in
	// evidence escalates, whatever autonomy its agent has earned — that is the
	// whole point, and an agent with UnlimitedActions is exactly the one this
	// is for. It names the source, because "which of these is somebody else's"
	// is the first thing a reviewer needs and the last thing the plan says.
	//
	// A plan with no actions does not escalate: there is nothing to approve,
	// and drafting is not acting. Same guard as the zero-cap rule below.
	if ev.HasThirdParty() && len(plannedCapabilities) > 0 {
		v.Escalate = true
		v.Reason = fmt.Sprintf("plan drew on material written outside this deployment: %s",
			strings.Join(ev.ThirdParty, ", "))
	}

	// 2. Confidence. A threshold of zero opts out rather than escalating
	// everything — an agent that declared no threshold has no opinion.
	if !v.Escalate && threshold > 0 && confidence < threshold {
		v.Escalate = true
		v.Reason = fmt.Sprintf("confidence %.2f below threshold %.2f", confidence, threshold)
	}

	// 3. Capabilities the bounds name as needing approval.
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

	// 4. A cap of zero is the "draft and ask" bound: no autonomous actions at
	// all. It escalates rather than trimming, for the reason on Verdict.Allowed.
	if !v.Escalate && b.MaxAutonomousActions == 0 && len(plannedCapabilities) > 0 {
		v.Escalate = true
		v.Reason = "agent is bounded to no autonomous actions"
	}

	// 5. Trim to the cap. UnlimitedActions (negative) and zero both opt out of
	// trimming — zero because it escalated above. The cap applies whatever
	// escalated: an approval falls through to act, so the reviewer must be
	// shown the plan that will actually run, and the cap is what will run.
	if b.MaxAutonomousActions > 0 && v.Allowed > b.MaxAutonomousActions {
		v.Allowed = b.MaxAutonomousActions
	}

	return v
}
