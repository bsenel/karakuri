package loop

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// State is the persistent slice of a running loop — enough information for a
// fresh server process to identify and resume work after a restart. Transient
// per-process resources (the checkpoint decision channel, the live agent
// instance) are NOT in here; they're rebuilt by the runner on resume.
//
// The storage.StorageAdapter interface declares Save/Get/List/Delete methods
// over this type; the runner persists at iteration boundaries so a server
// crash never loses more than one iteration of progress.
type State struct {
	LoopID      string                `json:"loop_id"`
	ObjectiveID objective.ObjectiveID `json:"objective_id"`
	TwinID      string                `json:"twin_id,omitempty"`
	AgentID     string                `json:"agent_id,omitempty"`

	Iteration    int                       `json:"iteration"`
	Paused       bool                      `json:"paused"`
	Completed    bool                      `json:"completed"`
	LastStep     Step                      `json:"last_step,omitempty"`
	Status       objective.ObjectiveStatus `json:"status,omitempty"`
	CriteriaMet  float64                   `json:"criteria_met"`
	CheckpointID string                    `json:"checkpoint_id,omitempty"`

	// Outcome is how the run ended, kept apart from Criteria: a run that spent
	// its budget has ended, and that says nothing about whether it succeeded.
	Outcome RunOutcome `json:"outcome,omitempty"`
	// Criteria are the states the last verify step settled, all read over the
	// same state of the world.
	Criteria []objective.Criterion `json:"criteria,omitempty"`
	// Error is the text of the error a run ended with, empty when it ended
	// without one. Text rather than the error value, because this row is read
	// by a process that did not run the loop (Phase 36: reconcile reads it to
	// tell a provider's rate limit from a failure).
	Error string `json:"error,omitempty"`
	// StoppedBy and StopReason are who stopped the run and why, set only when
	// it ended on a stop (Phase 37 step 3).
	StoppedBy  string `json:"stopped_by,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	// RetryAfter is how long the provider asked the run to wait, set only
	// when it ended waiting_for_quota and the provider said; zero otherwise.
	// Reconcile reads it as the time the quota returns (Phase 40 step 8).
	RetryAfter time.Duration `json:"retry_after,omitempty"`
	// BoundsViolation is the first action result of the run that carried one
	// (Phase 40 step 3), so the supervisor blocks the stream on a typed field
	// rather than on Error's text.
	BoundsViolation *environment.BoundsViolation `json:"bounds_violation,omitempty"`

	// PlanRefusedReason is why the authority bounds refused a plan during the
	// run (Phase 40 step 14a), empty when none was refused. A typed field, so
	// reconcile need not read the audit row's text.
	PlanRefusedReason string `json:"plan_refused_reason,omitempty"`

	// RequestJSON is the full marshalled Request as supplied to Run(); enough
	// to reconstruct the loop's parameters on a cold start.
	RequestJSON string `json:"request_json,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RunOutcome is how a run ended. It is never a criterion state: the verifiers
// settle those from the world the run left.
type RunOutcome string

const (
	RunConverged   RunOutcome = "converged"
	RunBudgetSpent RunOutcome = "budget_spent"
	RunErrored     RunOutcome = "errored"
	// RunStopped is a run somebody stopped from outside. It is not an error:
	// nothing in the run failed.
	RunStopped RunOutcome = "stopped"
	// RunInterruptedByRestart is a standing objective's loop that the process
	// died under, ended at boot rather than replayed (Phase 40 step 4). It is
	// not an error: reconcile re-runs the pass it cut off.
	RunInterruptedByRestart RunOutcome = "interrupted_by_restart"
	// RunRefusedByBounds is a run whose plan fell outside its bounds' action
	// limits at a rung that refuses rather than escalates (Phase 40 step 7).
	// It is not an error: the bounds did what they were declared to do.
	RunRefusedByBounds RunOutcome = "refused_by_bounds"
	// RunWaitingForQuota is a run whose delegated coding agent its provider
	// refused for quota (Phase 40 step 8). It is not an error: reconcile defers
	// the pass until the quota returns.
	RunWaitingForQuota RunOutcome = "waiting_for_quota"
)
