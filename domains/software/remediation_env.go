package software

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// CapRunRemediation runs a command against a named alert, for a stated reason.
const CapRunRemediation = "software.act.run_remediation"

// remediationEnv runs a remediation command through the shell executor, so the
// denylist, the workdir confinement and the timeout are the shell's own.
type remediationEnv struct {
	id    environment.EnvironmentID
	shell *shellEnv
}

func newRemediationEnv(id environment.EnvironmentID, sh *shellEnv) *remediationEnv {
	return &remediationEnv{id: id, shell: sh}
}

func (e *remediationEnv) ID() environment.EnvironmentID { return e.id }
func (e *remediationEnv) Domain() string                { return "software" }

func (e *remediationEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	state := map[string]any{}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

func (e *remediationEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return failureResult(e.id, a.CapabilityID, fmt.Sprintf("%s is not implemented", CapRunRemediation), nil), nil
}

func (e *remediationEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *remediationEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}
