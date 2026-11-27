package software

// The blockers the streams handed over (Phase 40 step 14c): one a stream could
// not remove after its declared attempts, or a kind that came back inside the
// declared window. The engineering_backlog stream observes them and files one
// Engineering Backlog entry per kind with the evidence.
//
// Scaffolding only: the tests in handed_blockers_test.go pin the behaviour,
// and until it is written the environment is blind, as AGENTS.md rule 10
// requires of one that cannot see.

import (
	"context"
	"errors"
	"fmt"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// EnvHandedBlockers is the environment the engineering_backlog stream reads
// the handed-over blockers from.
const EnvHandedBlockers environment.EnvironmentID = "software.env.handed_blockers"

// HandedBlocker is one kind of blocker handed to the backlog, across every
// stream that handed it, with its evidence.
type HandedBlocker struct {
	Kind string `json:"kind"`
	// Streams are the objectives that handed it over, sorted.
	Streams []string `json:"streams"`
	// Subjects are what it was about, sorted.
	Subjects  []string `json:"subjects"`
	Diagnosis string   `json:"diagnosis,omitempty"`
	// Attempts are every failed attempt, with its audit rows, oldest first.
	Attempts []reconcile.Attempt `json:"attempts,omitempty"`
	// Occurrences is how many times a blocker of the kind was cleared and came
	// back, summed over the streams.
	Occurrences int `json:"occurrences"`
}

// NewHandedBlockersEnvironment observes the handovers on the reconcile states
// read returns.
func NewHandedBlockersEnvironment(id environment.EnvironmentID, read func(context.Context) ([]reconcile.State, error)) environment.Environment {
	return &handedBlockersEnv{id: id, read: read}
}

type handedBlockersEnv struct {
	id   environment.EnvironmentID
	read func(context.Context) ([]reconcile.State, error)
}

func (e *handedBlockersEnv) ID() environment.EnvironmentID { return e.id }
func (e *handedBlockersEnv) Domain() string                { return "software" }

func (e *handedBlockersEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{}, errors.New("the handed-over blockers are not read yet (phase 40 step 14c)")
}

func (e *handedBlockersEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf("%s: the handed-blockers environment only observes", a.CapabilityID)}, nil
}

func (e *handedBlockersEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *handedBlockersEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}
