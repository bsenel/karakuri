package software

import (
	"context"
	"fmt"
	"strings"
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
	state := map[string]any{
		"adapter":      "remediation.exec",
		"capabilities": []string{CapRunRemediation},
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

// Act runs the command once it names its alert and its reason. Whether it may
// run at all is not decided here: that is the agent's AuthorityBounds (ADR 015).
func (e *remediationEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	if string(a.CapabilityID) != CapRunRemediation {
		return failureResult(e.id, a.CapabilityID,
			fmt.Sprintf("remediationEnv does not handle capability %q (only %s)", a.CapabilityID, CapRunRemediation), nil), nil
	}
	alertID, rationale := asString(a.Params, "alert_id"), asString(a.Params, "rationale")
	for _, p := range []struct{ name, value, what string }{
		{"alert_id", alertID, "the observed alert this remediation is for"},
		{"rationale", rationale, "why this command addresses that alert"},
		{"cmd", asString(a.Params, "cmd"), "the command to run"},
	} {
		if strings.TrimSpace(p.value) == "" {
			return failureResult(e.id, a.CapabilityID,
				fmt.Sprintf("%s needs params.%s: %s", CapRunRemediation, p.name, p.what), nil), nil
		}
	}
	// Reuse the shell executor wholesale, as verifyEnv does: the denylist, the
	// workdir confinement and the timeout are the same guarantees.
	res, err := e.shell.Act(ctx, environment.Action{
		CapabilityID: "software.act.shell_exec",
		Params:       a.Params,
	})
	if err != nil {
		return res, err
	}
	if res.StateDelta == nil {
		res.StateDelta = map[string]any{}
	}
	res.StateDelta["capability"] = CapRunRemediation
	res.StateDelta["alert_id"] = alertID
	res.StateDelta["rationale"] = rationale
	return res, nil
}

func (e *remediationEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *remediationEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}
