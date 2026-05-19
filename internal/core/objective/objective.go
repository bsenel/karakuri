package objective

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

type ObjectiveID string

type ObjectiveStatus string

const (
	StatusPending   ObjectiveStatus = "pending"
	StatusActive    ObjectiveStatus = "active"
	StatusBlocked   ObjectiveStatus = "blocked"
	StatusCompleted ObjectiveStatus = "completed"
	StatusFailed    ObjectiveStatus = "failed"
)

type Objective struct {
	ID              ObjectiveID     `json:"id"`
	Title           string          `json:"title"`
	Description     string          `json:"description,omitempty"`
	Domain          string          `json:"domain"`
	TwinID          string          `json:"twin_id,omitempty"`
	Priority        int             `json:"priority,omitempty"`
	Deadline        *time.Time      `json:"deadline,omitempty"`
	SuccessCriteria []Criterion     `json:"success_criteria,omitempty"`
	Constraints     []Constraint    `json:"constraints,omitempty"`
	ParentID        *ObjectiveID    `json:"parent_id,omitempty"`
	Status          ObjectiveStatus `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Criterion struct {
	ID          string                  `json:"id"`
	Description string                  `json:"description"`
	Verifier    capability.CapabilityID `json:"verifier,omitempty"`
	Threshold   any                     `json:"threshold,omitempty"`
	Weight      float64                 `json:"weight"`
	Met         bool                    `json:"met"`
}

type Constraint struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Hard        bool   `json:"hard"`
	Expression  string `json:"expression,omitempty"`
}
