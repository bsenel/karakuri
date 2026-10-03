package software

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

const (
	CapFetchLogs    = "software.observe.fetch_logs"
	CapFetchMetrics = "software.observe.fetch_metrics"
)

// errObservabilityNotImplemented marks the scaffolding. Phase 32 slice 4 lands
// the tests first; the behaviour replaces every use of this.
var errObservabilityNotImplemented = errors.New("observability environment: not implemented")

// observabilityEnv is the runtime as evidence: what is firing, and the logs and
// metrics behind it. adapter is nil when the twin binds no observability
// instance; there is no no-op adapter to fall back on, on purpose.
type observabilityEnv struct {
	id      environment.EnvironmentID
	adapter observability.ObservabilityAdapter
}

func newObservabilityEnv(id environment.EnvironmentID, adapter observability.ObservabilityAdapter) *observabilityEnv {
	return &observabilityEnv{id: id, adapter: adapter}
}

func (e *observabilityEnv) ID() environment.EnvironmentID { return e.id }
func (e *observabilityEnv) Domain() string                { return "software" }

func (e *observabilityEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{}, errObservabilityNotImplemented
}

func (e *observabilityEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, errObservabilityNotImplemented
}

func (e *observabilityEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *observabilityEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{}, errObservabilityNotImplemented
}

// observabilityFingerprint is the SHA Snapshot reports for a set of alerts.
func observabilityFingerprint([]observability.Alert) string {
	return ""
}
