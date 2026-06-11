package agriculture

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

func agricultureEnvironmentFactories() []environment.Factory {
	noop := func(id, desc string) environment.Factory {
		return environment.Factory{
			EnvID:       environment.EnvironmentID(id),
			Domain:      "agriculture",
			Description: desc,
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return &noopAgricultureEnv{id: environment.EnvironmentID(id)}, nil
			},
		}
	}
	return []environment.Factory{
		noop("agriculture.env.field", "Field sensor network: soil moisture, pH, nutrients, temperature, and zone actuators"),
		noop("agriculture.env.weather_api", "Weather data provider: current conditions, 14-day forecast, and historical records"),
	}
}

// noopAgricultureEnv is the default no-op environment returned when no real adapter is configured.
type noopAgricultureEnv struct {
	id environment.EnvironmentID
}

func (e *noopAgricultureEnv) ID() environment.EnvironmentID { return e.id }
func (e *noopAgricultureEnv) Domain() string                 { return "agriculture" }

func (e *noopAgricultureEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{
		EnvID:     e.id,
		State:     map[string]any{"status": "noop"},
		Version:   "noop-0",
		Timestamp: time.Now().UTC(),
	}, nil
}

// Act returns a failure for any capability invocation — the noop env
// has no implementation. See the analogous comment in the software
// pack for context on why honest failure beats fake success.
func (e *noopAgricultureEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("capability %q has no implementation: agriculture noop environment cannot execute", a.CapabilityID),
		StateDelta: map[string]any{
			"action": string(a.CapabilityID),
			"status": "unimplemented",
			"env_id": string(e.id),
		},
	}, nil
}

func (e *noopAgricultureEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *noopAgricultureEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{
		SHA:       "noop-snapshot",
		EnvID:     e.id,
		State:     map[string]any{"status": "noop"},
		Timestamp: time.Now().UTC(),
	}, nil
}
