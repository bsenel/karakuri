package software

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

const envRemediation = environment.EnvironmentID("software.env.remediation")

func newTestRemediationEnv() *remediationEnv {
	return newRemediationEnv(envRemediation, newShellEnv(envRemediation, "", 30*time.Second))
}

// A remediation names the alert it is for and why. One that cannot say either
// is a shell command with a different name, and it does not run.
func TestRemediationRefusesWithoutAlertRationaleOrCommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing string
	}{
		{"no alert", "alert_id"},
		{"no rationale", "rationale"},
		{"no command", "cmd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			params := map[string]any{
				"alert_id":  "a-1",
				"rationale": "restart the stuck worker",
				"cmd":       "touch " + marker,
			}
			params[tc.missing] = ""

			res, err := newTestRemediationEnv().Act(context.Background(), environment.Action{
				CapabilityID: CapRunRemediation, Params: params,
			})
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if res.Success {
				t.Fatalf("ran a remediation with no %s", tc.missing)
			}
			if !strings.Contains(res.Error, tc.missing) {
				t.Errorf("error %q does not name the missing param %s", res.Error, tc.missing)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Errorf("the command was executed although %s was missing", tc.missing)
			}
		})
	}
}

func TestRemediationRunsAndRecordsWhatItWasFor(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	res, err := newTestRemediationEnv().Act(context.Background(), environment.Action{
		CapabilityID: CapRunRemediation,
		Params: map[string]any{
			"alert_id":  "a-1",
			"rationale": "restart the stuck worker",
			"cmd":       "touch " + marker + " && echo restarted",
		},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("a valid remediation was refused: %s", res.Error)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the command did not run: %v", err)
	}
	for key, want := range map[string]any{
		"capability": CapRunRemediation,
		"alert_id":   "a-1",
		"rationale":  "restart the stuck worker",
		"exit_code":  0,
		"status":     "ok",
	} {
		if got := res.StateDelta[key]; got != want {
			t.Errorf("StateDelta[%s] = %v, want %v", key, got, want)
		}
	}
	if got, _ := res.StateDelta["stdout"].(string); !strings.Contains(got, "restarted") {
		t.Errorf("stdout = %q, want the command's output passed through", got)
	}
}

// The shell's denylist is the guarantee; a rationale does not buy a way round it.
func TestRemediationKeepsTheShellDenylist(t *testing.T) {
	res, err := newTestRemediationEnv().Act(context.Background(), environment.Action{
		CapabilityID: CapRunRemediation,
		Params: map[string]any{
			"alert_id":  "a-1",
			"rationale": "install the missing package",
			"cmd":       "sudo apt-get install foo",
		},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Fatal("a denylisted command ran as a remediation")
	}
	if !strings.Contains(res.Error, "safety guardrail") {
		t.Errorf("expected safety-guardrail error, got %q", res.Error)
	}
}

func TestRemediationRefusesOtherCapabilities(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	for _, capID := range []capability.CapabilityID{"software.act.shell_exec", "software.act.write_code"} {
		res, err := newTestRemediationEnv().Act(context.Background(), environment.Action{
			CapabilityID: capID,
			Params: map[string]any{
				"alert_id": "a-1", "rationale": "restart the stuck worker", "cmd": "touch " + marker,
			},
		})
		if err != nil {
			t.Fatalf("act: %v", err)
		}
		if res.Success {
			t.Errorf("the remediation environment accepted %s", capID)
		}
		if !strings.Contains(res.Error, string(capID)) {
			t.Errorf("error %q does not name the refused capability %s", res.Error, capID)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("the command was executed for %s", capID)
		}
	}
}

func TestRemediationObserveReportsItsCapability(t *testing.T) {
	obs, err := newTestRemediationEnv().Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.EnvID != envRemediation {
		t.Errorf("EnvID = %s, want %s", obs.EnvID, envRemediation)
	}
	if adapter, _ := obs.State["adapter"].(string); adapter == "" {
		t.Errorf("state does not name the adapter: %v", obs.State)
	}
	if got, want := obs.State["capabilities"], []string{CapRunRemediation}; !reflect.DeepEqual(got, want) {
		t.Errorf("capabilities = %v, want %v", got, want)
	}
}

func TestRemediationRoutesToItsEnvironment(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	if got := reg.ServedBy(CapRunRemediation); !reflect.DeepEqual(got, []environment.EnvironmentID{envRemediation}) {
		t.Errorf("%s is served by %v, want only %s", CapRunRemediation, got, envRemediation)
	}
}
