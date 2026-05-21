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
	LoopID      string                    `json:"loop_id"`
	ObjectiveID objective.ObjectiveID     `json:"objective_id"`
	TwinID      string                    `json:"twin_id,omitempty"`
	AgentID     string                    `json:"agent_id,omitempty"`

	Iteration    int                       `json:"iteration"`
	Paused       bool                      `json:"paused"`
	Completed    bool                      `json:"completed"`
	LastStep     Step                      `json:"last_step,omitempty"`
	Status       objective.ObjectiveStatus `json:"status,omitempty"`
	CriteriaMet  float64                   `json:"criteria_met"`
	CheckpointID string                    `json:"checkpoint_id,omitempty"`

	// RequestJSON is the full marshalled Request as supplied to Run(); enough
	// to reconstruct the loop's parameters on a cold start.
	RequestJSON string `json:"request_json,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
