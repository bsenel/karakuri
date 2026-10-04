package loop

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformobs "github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

const (
	incidentObsEnv   = "software.env.observability"
	incidentAlertID  = "alert-checkout-5xx"
	incidentInstance = "scripted-obs"
)

// scriptedObservability stands in for an alerting backend. It holds a set of
// alerts and reports one as resolved (drops it from the firing set) once its
// marker file exists, so the only thing that makes an alert go away is a
// remediation command that actually ran.
type scriptedObservability struct {
	alerts   []observability.Alert
	markers  map[string]string // alert ID → file whose existence resolves it
	inactive bool
}

func (o *scriptedObservability) Name() string { return incidentInstance }
func (o *scriptedObservability) Active() bool { return !o.inactive }

func (o *scriptedObservability) GetAlerts(context.Context, string, string, time.Time, string) ([]observability.Alert, error) {
	var out []observability.Alert
	for _, a := range o.alerts {
		if m := o.markers[a.ID]; m != "" {
			if _, err := os.Stat(m); err == nil {
				continue
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func (o *scriptedObservability) FetchLogs(context.Context, observability.LogQuery) ([]observability.LogLine, error) {
	return []observability.LogLine{{Time: time.Now().UTC(), Service: "checkout", Message: "upstream timed out"}}, nil
}

func (o *scriptedObservability) FetchMetrics(context.Context, observability.MetricQuery) ([]observability.MetricSeries, error) {
	return nil, nil
}

// incidentContext is the write-path harness with the scripted observability
// instance installed (obs nil: none installed) and bound (bind false: not
// bound), the pack's real SRE agent, an objective carrying the incident
// template's criteria, and the checkpoint service the decide step escalates
// through.
func incidentContext(t *testing.T, obs *scriptedObservability, bind bool) *stepContext {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	var bindings map[string]string
	if bind {
		bindings = map[string]string{"observability": incidentInstance}
	}
	sc := packContext(t, scratchRepo(t), &scriptedCLI{}, &recordingVC{}, func(reg *tools.Registry) {
		if obs != nil {
			reg.Observability.Set(incidentInstance, "stub", obs)
		}
	}, bindings)

	pack := software.New()
	var found bool
	for _, def := range pack.AgentDefinitions() {
		if def.ID == "software.agent.sre" {
			sc.agentDef, found = def, true
		}
	}
	if !found {
		t.Fatal("the software pack declares no software.agent.sre")
	}
	sc.obj = objective.Objective{ID: "obj-incident", Title: "checkout is returning 5xx", Domain: "software", TwinID: "twin-1"}
	for _, tmpl := range pack.ObjectiveTemplates() {
		if tmpl.ID == "software.objective.incident_response" {
			sc.obj.TemplateID = "software.objective.incident_response"
			sc.obj.SuccessCriteria = append([]objective.Criterion(nil), tmpl.SuccessCriteria...)
		}
	}
	if len(sc.obj.SuccessCriteria) == 0 {
		t.Fatal("the software pack declares no software.objective.incident_response criteria")
	}
	if err := sc.svc.store.SaveObjective(context.Background(), sc.obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}

	sc.loopID = "loop-incident-0001"
	sc.iteration = 1
	sc.state = &loopState{id: sc.loopID, decisionCh: make(chan corecheckpoint.Decision, 1)}
	cpSvc := featurecp.NewService(sc.svc.store, sc.svc.hub)
	sc.svc.cpSvc = cpSvc
	sc.svc.states = map[string]*loopState{sc.loopID: sc.state}
	sc.svc.memSvc = featurememory.NewService(sc.svc.store, 5)
	sc.svc.otel = platformobs.NewOTel(nil)
	cpSvc.SetResumer(sc.svc)
	return sc
}

func firingAlert() observability.Alert {
	return observability.Alert{
		ID: incidentAlertID, Service: "checkout", Severity: "critical",
		Message: "checkout 5xx rate above 5% for 10m", State: observability.AlertFiring,
		Time: time.Now().UTC().Add(-10 * time.Minute),
	}
}

// incidentPlan is the plan an SRE agent drafts for one alert: look, remediate,
// check. cmd is the remediation command.
func incidentPlan(cmd string) plan {
	return plan{
		Confidence: 0.95,
		Actions: []plannedAction{
			{CapabilityID: software.CapFetchLogs, Params: map[string]any{"service": "checkout"}},
			{CapabilityID: software.CapRunRemediation, Params: map[string]any{
				"alert_id":  incidentAlertID,
				"rationale": "the upstream pool is exhausted; restarting it clears the 5xx",
				"cmd":       cmd,
			}},
			{CapabilityID: software.CapAlertsResolved, Params: map[string]any{"alert_ids": []string{incidentAlertID}}},
		},
	}
}

func outcomeOf(t *testing.T, outcomes []actionOutcome, capID string) actionOutcome {
	t.Helper()
	for _, o := range outcomes {
		if o.CapabilityID == capID {
			return o
		}
	}
	t.Fatalf("no outcome for %s in %+v", capID, outcomes)
	return actionOutcome{}
}

func observationFrom(obs []environment.Observation, id string) (environment.Observation, bool) {
	for _, o := range obs {
		if string(o.EnvID) == id {
			return o, true
		}
	}
	return environment.Observation{}, false
}

func listed(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// The Phase 32 acceptance sentence, stage by stage: an incident is observed,
// its remediation is escalated to a human, approved, run, and recorded.
//
// NOT YET WRITTEN in this file: A5 (the digest includes the objective and the
// decision) and A6 (the incident template's criteria settle from the
// alerts_resolved outcome, and do not when the command clears nothing).
func TestIncidentObservedEscalatedRemediatedRecorded(t *testing.T) {
	ctx := context.Background()
	marker := filepath.Join(t.TempDir(), "remediated")
	obs := &scriptedObservability{
		alerts:  []observability.Alert{firingAlert()},
		markers: map[string]string{incidentAlertID: marker},
	}
	sc := incidentContext(t, obs, true)
	p := incidentPlan("touch " + marker)

	t.Run("A1 observe", func(t *testing.T) {
		ws := stepObserve(ctx, sc)
		sc.observed = &ws
		if listed(ws.Blind, incidentObsEnv) {
			t.Fatalf("%s is blind with an active instance bound: %v", incidentObsEnv, ws.Blind)
		}
		o, ok := observationFrom(ws.Observations, incidentObsEnv)
		if !ok {
			t.Fatalf("no observation from %s", incidentObsEnv)
		}
		raw, _ := json.Marshal(o.State)
		if !strings.Contains(string(raw), incidentAlertID) {
			t.Errorf("the observation does not carry the firing alert %s: %s", incidentAlertID, raw)
		}
		if o.Trust != environment.TrustThirdParty {
			t.Errorf("Trust = %q, want third-party: an alert message is somebody's prose", o.Trust)
		}
		if !listed(sc.evidence.ThirdParty, incidentObsEnv) {
			t.Errorf("evidence third-party sources = %v, want %s among them", sc.evidence.ThirdParty, incidentObsEnv)
		}
	})

	var cpID string
	t.Run("A2 propose and escalate", func(t *testing.T) {
		_, paused := stepDecide(ctx, sc, p, nil)
		if !paused {
			t.Fatal("a plan naming software.act.run_remediation was not escalated under the SRE agent's bounds")
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("the remediation ran before anybody approved it")
		}
		if sc.state.result.CheckpointID == nil || *sc.state.result.CheckpointID == "" {
			t.Fatal("the escalation created no checkpoint")
		}
		cpID = *sc.state.result.CheckpointID

		// Why, not merely that: the gate that must hold for a remediation is
		// RequiresApprovalFor, and the audit row has to say so.
		rows, err := sc.svc.store.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
		if err != nil {
			t.Fatalf("list tool events: %v", err)
		}
		var reason string
		for _, r := range rows {
			if r.Kind == storage.ToolEventEscalation {
				reason = r.EscalationReason
			}
		}
		if !strings.Contains(reason, software.CapRunRemediation) {
			t.Errorf("escalation reason %q does not name the requires-approval capability %s", reason, software.CapRunRemediation)
		}
	})

	t.Run("A3 approve and act", func(t *testing.T) {
		if cpID == "" {
			t.Fatal("no checkpoint to approve: stage A2 did not escalate")
		}
		if err := sc.svc.cpSvc.Resolve(ctx, cpID, corecheckpoint.Decision{Choice: "approve", Approver: "ada", Note: "restart it"}); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		select {
		case d := <-sc.state.decisionCh:
			if d.Choice != "approve" || d.Approver != "ada" {
				t.Fatalf("delivered %+v, want ada's approval", d)
			}
		default:
			t.Fatal("approving the checkpoint did not deliver a decision to the waiting loop")
		}

		outcomes := stepAct(ctx, sc, p)
		if len(outcomes) != 3 {
			t.Fatalf("got %d outcomes, want 3: %+v", len(outcomes), outcomes)
		}
		if o := outcomeOf(t, outcomes, software.CapFetchLogs); !o.Result.Success {
			t.Errorf("fetch_logs failed: %s", o.Result.Error)
		}
		rem := outcomeOf(t, outcomes, software.CapRunRemediation)
		if !rem.Result.Success {
			t.Errorf("run_remediation failed: %s", rem.Result.Error)
		}
		if got := rem.Result.StateDelta["alert_id"]; got != incidentAlertID {
			t.Errorf("run_remediation result names alert %v, want %s", got, incidentAlertID)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("the remediation command did not run: %v", err)
		}
		if o := outcomeOf(t, outcomes, software.CapAlertsResolved); !o.Result.Success {
			t.Errorf("alerts_resolved failed after the remediation ran: %s", o.Result.Error)
		}
	})

	t.Run("A4 audit", func(t *testing.T) {
		rows, err := sc.svc.store.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
		if err != nil {
			t.Fatalf("list tool events: %v", err)
		}
		caps := []string{software.CapFetchLogs, software.CapRunRemediation, software.CapAlertsResolved}
		var escalated, approved bool
		executed := map[string]bool{}
		for _, r := range rows {
			raw, _ := json.Marshal(r)
			switch r.Kind {
			case storage.ToolEventEscalation:
				escalated = true
			case storage.ToolEventApproval:
				approved = approved || strings.Contains(string(raw), "ada")
			case storage.ToolEventExecute, "":
				for _, c := range caps {
					if strings.Contains(string(raw), c) {
						executed[c] = r.Success
					}
				}
			}
		}
		if !escalated {
			t.Error("tool_events holds no escalation row for the objective")
		}
		if !approved {
			t.Error("tool_events holds no approval row naming the approver ada")
		}
		for _, c := range caps {
			if _, ok := executed[c]; !ok {
				t.Errorf("tool_events holds no execute row for %s", c)
			}
		}
		if !executed[software.CapRunRemediation] {
			t.Error("the run_remediation execute row is not marked successful")
		}
	})
}

// A deployment that binds no observability instance, or binds one that is not
// active, cannot see. Blind is never quiet and never resolved.
//
// B2 (reconcile's sensing lists the environment as blind and produces NO SHA
// for it) is NOT in this file: reconcile's sensing function is unexported and
// cannot be reached from package loop, so it belongs in
// internal/feature/reconcile with a name containing Blind.
func TestIncidentBlindWhenObservabilityUnbound(t *testing.T) {
	cases := map[string]struct {
		obs  *scriptedObservability
		bind bool
	}{
		"no binding":        {obs: nil, bind: false},
		"inactive instance": {obs: &scriptedObservability{alerts: []observability.Alert{firingAlert()}, inactive: true}, bind: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			sc := incidentContext(t, tc.obs, tc.bind)

			// B1: named as blind, and no observation in its place.
			ws := stepObserve(ctx, sc)
			if !listed(ws.Blind, incidentObsEnv) {
				t.Errorf("Blind = %v, want %s: it cannot see", ws.Blind, incidentObsEnv)
			}
			if _, ok := observationFrom(ws.Observations, incidentObsEnv); ok {
				t.Errorf("%s produced an observation while blind", incidentObsEnv)
			}

			// B3: the verifier cannot look, so it fails and the criterion it
			// settles is not met.
			outcomes := stepAct(ctx, sc, plan{Confidence: 0.95, Actions: []plannedAction{{
				CapabilityID: software.CapAlertsResolved,
				Params:       map[string]any{"alert_ids": []string{incidentAlertID}},
			}}})
			if len(outcomes) != 1 {
				t.Fatalf("got %d outcomes, want 1", len(outcomes))
			}
			if outcomes[0].Result.Success {
				t.Fatal("alerts_resolved succeeded on a deployment that cannot look: blind was read as resolved")
			}
			// Only the verifier-backed criterion is scored here; root-cause is
			// judged by a model and is not what this case is about.
			var remediation []objective.Criterion
			for _, c := range sc.obj.SuccessCriteria {
				if c.ID == "remediation" {
					remediation = append(remediation, c)
				}
			}
			if len(remediation) != 1 {
				t.Fatalf("the incident template has %d remediation criteria, want 1", len(remediation))
			}
			sc.obj.SuccessCriteria = remediation
			if score, met := stepVerify(ctx, sc, outcomes); met || score != 0 {
				t.Errorf("remediation criterion scored %.2f (met=%v) on a deployment that cannot look", score, met)
			}
		})
	}
}
