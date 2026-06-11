package checkpoint

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusResolved Status = "resolved"
)

// Action is the planner draft a reviewer sees on a pending checkpoint.
// Mirrors the loop package's internal plannedAction in a stable, public
// shape so the API + frontend can render the proposal without importing
// loop internals.
type Action struct {
	CapabilityID string         `json:"capability"`
	Params       map[string]any `json:"params,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	EnvID        string         `json:"env_id,omitempty"`
}

type Checkpoint struct {
	ID          string                  `json:"id"`
	ObjectiveID objective.ObjectiveID   `json:"objective_id"`
	TwinID      string                  `json:"twin_id"`
	Reason      string                  `json:"reason,omitempty"`
	Summary     string                  `json:"summary"`
	Options     []string                `json:"options"`
	Capability  capability.CapabilityID `json:"capability,omitempty"`
	Confidence  float64                 `json:"confidence,omitempty"`
	// Actions is the planner draft the agent proposed at the moment of
	// escalation. Populated by the loop runner so reviewers can judge what
	// they are approving without leaving the checkpoint response.
	Actions []Action `json:"actions,omitempty"`
	// AuditEventID links the checkpoint to the kind=escalation row in the
	// audit log that captured the full escalation payload. Empty when the
	// audit write failed; reviewers can still resolve.
	AuditEventID string     `json:"audit_event_id,omitempty"`
	Status       Status     `json:"status"`
	Decision     *Decision  `json:"decision,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// Modifications carries the structured edits a reviewer applies when they
// resolve a checkpoint with Choice="modify". The runner uses these to
// trim the draft and feed Reflexion-style critique input back into a
// single revise pass before the act step.
type Modifications struct {
	// RemovedActions lists capability IDs to drop from the draft. Each
	// match is removed once; duplicates in the draft after the first match
	// are kept.
	RemovedActions []string `json:"removed_actions,omitempty"`
	// AddedConstraints is free-text guidance fed into the revise pass as
	// critique input. One bullet per entry.
	AddedConstraints []string `json:"added_constraints,omitempty"`
	// RevisedConfidence, when non-nil, asserts an operator-set confidence
	// for THIS iteration. The runner treats this as both the plan's
	// effective confidence (bypassing the procedural-memory bias) AND a
	// per-iteration lowering of the bounds-check threshold. Together
	// these two semantics let an operator say "I'll accept this plan at
	// 0.85 — let it run" even when the agent's authority threshold is
	// 0.90. The override is per-iteration only; the agent's stated
	// threshold returns on the next iteration. Recorded on the
	// kind=modification audit row for attribution.
	RevisedConfidence *float64 `json:"revised_confidence,omitempty"`
}

type Decision struct {
	Choice        string         `json:"choice"`
	Note          string         `json:"note,omitempty"`
	Approver      string         `json:"approver,omitempty"` // operator name/id for the audit trail
	Modifications *Modifications `json:"modifications,omitempty"`
}

type Event struct {
	ID          string
	ObjectiveID objective.ObjectiveID
	Summary     string
	Options     []string
	Timestamp   time.Time
}
