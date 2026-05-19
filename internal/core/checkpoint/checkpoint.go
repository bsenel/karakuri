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

type Checkpoint struct {
	ID          string                `json:"id"`
	ObjectiveID objective.ObjectiveID `json:"objective_id"`
	TwinID      string                `json:"twin_id"`
	Reason      string                `json:"reason,omitempty"`
	Summary     string                `json:"summary"`
	Options     []string              `json:"options"`
	Capability  capability.CapabilityID `json:"capability,omitempty"`
	Confidence  float64               `json:"confidence,omitempty"`
	Status      Status                `json:"status"`
	Decision    *Decision             `json:"decision,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	ResolvedAt  *time.Time            `json:"resolved_at,omitempty"`
}

type Decision struct {
	Choice string `json:"choice"`
	Note   string `json:"note,omitempty"`
}

type Event struct {
	ID          string
	ObjectiveID objective.ObjectiveID
	Summary     string
	Options     []string
	Timestamp   time.Time
}
