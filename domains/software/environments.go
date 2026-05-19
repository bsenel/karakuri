package software

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

func softwareEnvironmentFactories() []environment.Factory {
	noop := func(id, desc string) environment.Factory {
		return environment.Factory{
			EnvID:       environment.EnvironmentID(id),
			Domain:      "software",
			Description: desc,
			Build: func(_ map[string]any) (environment.Environment, error) {
				return &noopEnv{id: environment.EnvironmentID(id)}, nil
			},
		}
	}
	return []environment.Factory{
		noop("software.env.git", "Git repository: commits, branches, PRs, worktrees, diffs"),
		noop("software.env.ci", "CI pipeline: build status, test results, coverage"),
		noop("software.env.observability", "Runtime: logs, metrics, alerts"),
		noop("software.env.codebase", "Static analysis: file tree, symbols, dependency graph"),
		noop("software.env.ticket", "Project management: issues, epics, sprints"),
		noop("software.env.communication", "Team signals: messages, threads, mentions"),
	}
}

// noopEnv is the default no-op environment returned when no real adapter is configured.
type noopEnv struct {
	id environment.EnvironmentID
}

func (e *noopEnv) ID() environment.EnvironmentID { return e.id }
func (e *noopEnv) Domain() string                 { return "software" }

func (e *noopEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{
		EnvID: e.id, State: map[string]any{"status": "noop"},
		Version: "noop-0", Timestamp: time.Now().UTC(),
	}, nil
}

func (e *noopEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{
		Success:    true,
		StateDelta: map[string]any{"action": string(a.CapabilityID), "status": "noop"},
	}, nil
}

func (e *noopEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *noopEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{
		SHA: "noop-snapshot", EnvID: e.id,
		State: map[string]any{"status": "noop"}, Timestamp: time.Now().UTC(),
	}, nil
}
