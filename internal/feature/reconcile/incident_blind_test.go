package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

type inactiveObservability struct{}

func (inactiveObservability) Name() string { return "down" }
func (inactiveObservability) Active() bool { return false }
func (inactiveObservability) GetAlerts(context.Context, string, string, time.Time, string) ([]observability.Alert, error) {
	return nil, nil
}
func (inactiveObservability) FetchLogs(context.Context, observability.LogQuery) ([]observability.LogLine, error) {
	return nil, nil
}
func (inactiveObservability) FetchMetrics(context.Context, observability.MetricQuery) ([]observability.MetricSeries, error) {
	return nil, nil
}

// Phase 32, stage B2 of the acceptance test in
// internal/feature/loop/incident_test.go (it lives here because fingerprint is
// unexported). The roadmap: "an environment that cannot see must not return a
// still snapshot, because Phase 20 will read that as a quiet world and stop
// looking". So the software pack's observability environment, unbound or bound
// to an inactive instance, is named blind and the fingerprint has no SHA.
func TestBlindObservabilityYieldsNoFingerprint(t *testing.T) {
	const envID = "software.env.observability"
	cases := map[string]map[string]string{
		"no binding":        nil,
		"inactive instance": {"observability": "down"},
	}
	for name, bindings := range cases {
		t.Run(name, func(t *testing.T) {
			reg := &tools.Registry{}
			reg.Observability.Set("down", "stub", inactiveObservability{})
			var envs []environment.Environment
			for _, f := range software.NewWithTools(reg).EnvironmentFactories() {
				if string(f.EnvID) != envID {
					continue
				}
				env, err := f.Build(environment.BuildContext{TwinID: "twin-1", AdapterBindings: bindings})
				if err != nil {
					t.Fatalf("build %s: %v", envID, err)
				}
				envs = append(envs, env)
			}
			if len(envs) != 1 {
				t.Fatalf("the software pack built %d %s environments, want 1", len(envs), envID)
			}

			fp := fingerprint(context.Background(), envs)
			if len(fp.Blind) != 1 || fp.Blind[0] != envID {
				t.Errorf("Blind = %v, want [%s]", fp.Blind, envID)
			}
			if sha, ok := fp.Environments[envID]; ok {
				t.Errorf("a blind environment contributed snapshot SHA %q", sha)
			}
			if fp.SHA != "" {
				t.Errorf("fingerprint SHA = %q, want the empty string: blind is not still", fp.SHA)
			}
		})
	}
}
