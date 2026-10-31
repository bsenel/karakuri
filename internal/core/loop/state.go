package loop

import (
	"time"

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
)
