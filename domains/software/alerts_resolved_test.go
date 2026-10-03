package software

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

func actAlertsResolved(t *testing.T, adapter observability.ObservabilityAdapter, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
		CapabilityID: CapAlertsResolved, Params: params,
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	return res
}

func assertAlertsResolvedDelta(t *testing.T, res environment.ActionResult, resolved, stillOpen []string) {
	t.Helper()
	if got := res.StateDelta["verifier"]; got != CapAlertsResolved {
		t.Errorf("verifier = %v, want %s", got, CapAlertsResolved)
	}
	for key, want := range map[string][]string{"resolved": resolved, "still_open": stillOpen} {
		got, ok := res.StateDelta[key].([]string)
		if !ok {
			t.Errorf("StateDelta[%s] is %T, want []string", key, res.StateDelta[key])
			continue
		}
		if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// The criterion "the alert is gone" is settled by asking the instance what is
// open, not by a model reading its own remediation output.
func TestAlertsResolvedWhenTheAlertIsNoLongerOpen(t *testing.T) {
	adapter := &fakeObservability{name: "prod", active: true, alerts: []observability.Alert{
		obsAlert("a-other", "critical", observability.AlertFiring),
	}}
	res := actAlertsResolved(t, adapter, map[string]any{"alert_ids": []string{"a-2", "a-1"}})
	if !res.Success {
		t.Fatalf("alerts that are not open were not reported resolved: %s", res.Error)
	}
	assertAlertsResolvedDelta(t, res, []string{"a-1", "a-2"}, nil)
	if res.Trust != environment.TrustOperator {
		t.Errorf("Trust = %q, want %q: the result carries IDs the caller passed, not alert text", res.Trust, environment.TrustOperator)
	}
	if adapter.alertCalls == 0 {
		t.Error("reported resolved without asking the adapter")
	}
}

// Acknowledged is somebody looking at it, not it being over.
func TestAlertsResolvedFailsWhileAnAlertIsOpen(t *testing.T) {
	for _, state := range []string{observability.AlertFiring, observability.AlertAcknowledged} {
		t.Run(state, func(t *testing.T) {
			adapter := &fakeObservability{name: "prod", active: true, alerts: []observability.Alert{
				obsAlert("a-9", "critical", state),
				obsAlert("a-3", "warning", state),
			}}
			res := actAlertsResolved(t, adapter, map[string]any{"alert_ids": []string{"a-9", "a-1", "a-3"}})
			if res.Success {
				t.Fatalf("reported resolved while alerts are %s", state)
			}
			assertAlertsResolvedDelta(t, res, []string{"a-1"}, []string{"a-3", "a-9"})
			for _, id := range []string{"a-3", "a-9"} {
				if !strings.Contains(res.Error, id) {
					t.Errorf("error %q does not name still-open alert %s", res.Error, id)
				}
			}
			if res.Trust != environment.TrustOperator {
				t.Errorf("Trust = %q, want %q", res.Trust, environment.TrustOperator)
			}
		})
	}
}

// Nothing named is nothing verified: "none of zero alerts are open" must not
// settle a criterion, and there is no reason to spend a call finding out.
func TestAlertsResolvedRefusesWithoutAlertIDs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"missing", map[string]any{}},
		{"nil params", nil},
		{"empty string", map[string]any{"alert_ids": ""}},
		{"blank string", map[string]any{"alert_ids": " , "}},
		{"empty list", map[string]any{"alert_ids": []string{}}},
		{"empty any list", map[string]any{"alert_ids": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &fakeObservability{name: "prod", active: true}
			res := actAlertsResolved(t, adapter, tc.params)
			if res.Success {
				t.Fatal("reported resolved with no alert named")
			}
			if !strings.Contains(res.Error, "alert_ids") {
				t.Errorf("error %q does not name the param alert_ids", res.Error)
			}
			if adapter.alertCalls != 0 {
				t.Errorf("GetAlerts was called %d times for a refused request", adapter.alertCalls)
			}
		})
	}
}

// Not being able to look is never the same as the alert being gone.
func TestAlertsResolvedNeverResolvesWhatItCouldNotLookAt(t *testing.T) {
	boom := errors.New("datadog: 429 rate limited")
	params := map[string]any{"alert_ids": []string{"a-1"}}

	for _, tc := range []struct {
		name    string
		adapter observability.ObservabilityAdapter
		want    string
	}{
		{"unbound", nil, "no observability instance is bound to this twin"},
		{"inactive", &fakeObservability{name: "prod"}, "not active"},
		{"alerts error", &fakeObservability{name: "prod", active: true, alertsErr: boom}, boom.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := actAlertsResolved(t, tc.adapter, params)
			if res.Success {
				t.Fatal("reported resolved for alerts that were not looked at")
			}
			if !strings.Contains(res.Error, "could not") {
				t.Errorf("error %q does not say it could not look", res.Error)
			}
			if !strings.Contains(res.Error, tc.want) {
				t.Errorf("error %q does not contain %q", res.Error, tc.want)
			}
			if got, ok := res.StateDelta["resolved"].([]string); ok && len(got) > 0 {
				t.Errorf("resolved = %v for alerts that were not looked at", got)
			}
			if f, ok := tc.adapter.(*fakeObservability); ok && !f.active && f.alertCalls != 0 {
				t.Error("an inactive adapter was called")
			}
		})
	}
}

// Models pass lists as lists, as []any off JSON, and as one comma-separated
// string. All three name the same alerts.
func TestAlertsResolvedAcceptsListOrCommaSeparatedIDs(t *testing.T) {
	alerts := []observability.Alert{obsAlert("a-2", "critical", observability.AlertFiring)}

	var results []environment.ActionResult
	for _, ids := range []any{
		[]string{"a-2", "a-1"},
		[]any{"a-2", "a-1"},
		"a-2, a-1",
	} {
		adapter := &fakeObservability{name: "prod", active: true, alerts: alerts}
		res := actAlertsResolved(t, adapter, map[string]any{"alert_ids": ids})
		if res.Success {
			t.Errorf("alert_ids %#v: reported resolved while a-2 is firing", ids)
		}
		assertAlertsResolvedDelta(t, res, []string{"a-1"}, []string{"a-2"})
		results = append(results, res)
	}
	for i, res := range results[1:] {
		if !reflect.DeepEqual(res, results[0]) {
			t.Errorf("form %d gave %+v, want the same as the string list: %+v", i+1, res, results[0])
		}
	}
}

func TestAlertsResolvedRoutesToObservability(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	if got := reg.ServedBy(CapAlertsResolved); !reflect.DeepEqual(got, []environment.EnvironmentID{envObservability}) {
		t.Errorf("%s is served by %v, want only %s", CapAlertsResolved, got, envObservability)
	}
}
