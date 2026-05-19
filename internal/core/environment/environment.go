package environment

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

type EnvironmentID string

type Environment interface {
	ID() EnvironmentID
	Domain() string

	Observe(ctx context.Context, q ObservationQuery) (Observation, error)
	Act(ctx context.Context, a Action) (ActionResult, error)
	Subscribe(ctx context.Context, f EventFilter) (<-chan EnvironmentEvent, error)
	Snapshot(ctx context.Context) (EnvironmentSnapshot, error)
}

type ObservationQuery struct {
	Filter map[string]any
	Limit  int
}

type Observation struct {
	EnvID     EnvironmentID
	State     map[string]any
	Version   string // SHA of this observation
	Timestamp time.Time
}

type Action struct {
	CapabilityID capability.CapabilityID
	Params       map[string]any
}

type ActionResult struct {
	Success      bool
	StateDelta   map[string]any
	ArtifactSHAs []string // any VFS blobs produced
	Error        string
}

type EventFilter struct {
	Kinds []string
}

type EnvironmentEvent struct {
	EnvID     EnvironmentID
	Kind      string
	Delta     map[string]any
	Timestamp time.Time
}

type EnvironmentSnapshot struct {
	SHA       string
	EnvID     EnvironmentID
	State     map[string]any
	Timestamp time.Time
}
