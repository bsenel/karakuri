package twin

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

type Kind string

const (
	KindPerson       Kind = "person"
	KindTeam         Kind = "team"
	KindOrganization Kind = "organization"
)

type DigitalTwin struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Kind         Kind                      `json:"kind"`
	Domain       string                    `json:"domain"`
	Agents       []agent.Definition        `json:"agents,omitempty"`
	Environments []environment.EnvironmentID `json:"environments,omitempty"`
	Objectives   []objective.ObjectiveID   `json:"objectives,omitempty"`
	Memory       agent.MemoryConfig        `json:"memory,omitempty"`
	Children     []string                  `json:"children,omitempty"`
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
}
