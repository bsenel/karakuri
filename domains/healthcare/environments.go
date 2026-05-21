package healthcare

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// healthcareEnvironmentFactories returns three environment factories with
// no-op default implementations:
//
//   - healthcare.env.ehr        — Electronic Health Records (problem list, meds,
//                                  allergies, vitals, encounter notes)
//   - healthcare.env.lab        — Laboratory information system (orders, results)
//   - healthcare.env.guidelines — Clinical-guideline reference store (NICE, ATS,
//                                  NCCN, etc.) consumed at the verify step
//
// Real EHR/lab adapters belong in the tools registry (versioncontrol-style
// slots could be added in a follow-up). For now the pack ships pluggable
// envs whose no-op defaults let the conformance suite + integration tests
// run without external systems.
func healthcareEnvironmentFactories() []environment.Factory {
	noop := func(id, desc string) environment.Factory {
		return environment.Factory{
			EnvID:       environment.EnvironmentID(id),
			Domain:      "healthcare",
			Description: desc,
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return &noopHealthcareEnv{id: environment.EnvironmentID(id)}, nil
			},
		}
	}
	return []environment.Factory{
		noop("healthcare.env.ehr",
			"Electronic Health Records: problem list, medications, allergies, vitals, encounters"),
		noop("healthcare.env.lab",
			"Laboratory information system: place orders and retrieve results"),
		noop("healthcare.env.guidelines",
			"Clinical-guideline reference store (NICE, ATS, NCCN, …) consumed at verify"),
	}
}

// noopHealthcareEnv is the inert default returned when no real adapter is
// wired — Observe returns a placeholder, Act records the attempt without
// executing anything externally. Production deployments either replace this
// with a real client or run the loop in dry-run mode.
type noopHealthcareEnv struct {
	id environment.EnvironmentID
}

func (e *noopHealthcareEnv) ID() environment.EnvironmentID { return e.id }
func (e *noopHealthcareEnv) Domain() string                { return "healthcare" }

func (e *noopHealthcareEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{
		EnvID:     e.id,
		State:     map[string]any{"status": "noop"},
		Version:   "noop-0",
		Timestamp: time.Now().UTC(),
	}, nil
}

func (e *noopHealthcareEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{
		Success:    true,
		StateDelta: map[string]any{"action": string(a.CapabilityID), "status": "noop"},
	}, nil
}

func (e *noopHealthcareEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *noopHealthcareEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{
		SHA:       "noop-snapshot",
		EnvID:     e.id,
		State:     map[string]any{"status": "noop"},
		Timestamp: time.Now().UTC(),
	}, nil
}
