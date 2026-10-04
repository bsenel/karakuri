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
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	"github.com/bsenel/karakuri/internal/feature/report"
	platformobs "github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
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
	sc.obj = objective.Objective{ID: "obj-incident", Title: "checkout is returning 5xx", Domain: "software", TwinID: "twin-1", Mode: objective.ModeStanding}
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
// The stages, in the order they run: A1 observe, A2 propose and escalate, A2b
// the approval list alone is enough, A5 digest before, A3 approve and act, A4
// audit, A5b digest after, A6 verify.
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
	var acted []actionOutcome
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

		// Why, not merely that: an incident plan is always drafted with an
		// alert in evidence, so it escalates on provenance before the approval
		// list is ever consulted. That is the stronger gate, and it holds
		// whatever the agent's bounds say. Decide reports one reason, the most
		// specific, so the audit row names the third-party source.
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
		if !strings.Contains(reason, incidentObsEnv) {
			t.Errorf("escalation reason %q does not name the third-party source %s", reason, incidentObsEnv)
		}
	})

	// Two independent gates, either one sufficient. This one is the gate that
	// still holds for a remediation proposed with no alert text in evidence:
	// the same agent, the same planned capabilities, nothing third-party, and
	// a confidence its own threshold would let through.
	t.Run("A2b the approval list alone is enough", func(t *testing.T) {
		b := sc.agentDef.Authority
		if b.ConfidenceThreshold >= 0.99 {
			t.Fatalf("the SRE's confidence threshold is %.2f: a 0.99 plan would escalate for confidence and this stage would prove nothing",
				b.ConfidenceThreshold)
		}
		planned := make([]capability.CapabilityID, 0, len(p.Actions))
		for _, a := range p.Actions {
			planned = append(planned, capability.CapabilityID(a.CapabilityID))
		}
		v := b.Decide(0.99, b.ConfidenceThreshold, planned, coreagent.Evidence{})
		if !v.Escalate {
			t.Fatalf("the SRE planned %s with nothing third-party in evidence and ran it without asking", software.CapRunRemediation)
		}
		if !strings.Contains(v.Reason, software.CapRunRemediation) {
			t.Errorf("escalation reason %q does not name the requires-approval capability %s", v.Reason, software.CapRunRemediation)
		}
	})

	// A5 runs while the checkpoint is still pending: a digest lists what the
	// reader owes an answer on, and once ada answers it is no longer owed.
	// A5b assembles the same window again after she has.
	digestSince := time.Now().UTC().Add(-time.Hour)
	t.Run("A5 digest, before: the decision is owed", func(t *testing.T) {
		if cpID == "" {
			t.Fatal("no checkpoint to report: stage A2 did not escalate")
		}
		d, err := report.NewService(sc.svc.store, nil, nil, karakuriquota.Deps{}, report.Config{}).
			Assemble(ctx, sc.twinID, digestSince, digestSince.Add(2*time.Hour))
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		var hasObjective, hasDecision bool
		for _, o := range d.Objectives {
			hasObjective = hasObjective || o.ID == sc.obj.ID
		}
		for _, dec := range d.Decisions {
			if dec.CheckpointID == cpID && dec.ObjectiveID == sc.obj.ID && listed(dec.Proposed, software.CapRunRemediation) {
				hasDecision = true
			}
		}
		if !hasObjective {
			t.Errorf("the digest does not include objective %s: %+v", sc.obj.ID, d.Objectives)
		}
		if !hasDecision {
			t.Errorf("the digest does not list checkpoint %s proposing %s: %+v", cpID, software.CapRunRemediation, d.Decisions)
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
		acted = outcomes
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

	// "Once approved, records the action in the audit log and in the next
	// digest": A4 was the audit log, this is the digest. Three, because the
	// plan has three actions and A4 found an execute row for each; the digest
	// counts execute rows, so looking and checking count beside the
	// remediation itself.
	t.Run("A5b digest, after: the action is recorded", func(t *testing.T) {
		if len(acted) == 0 {
			t.Fatal("no actions to report: stage A3 did not act")
		}
		d, err := report.NewService(sc.svc.store, nil, nil, karakuriquota.Deps{}, report.Config{}).
			Assemble(ctx, sc.twinID, digestSince, digestSince.Add(2*time.Hour))
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		var found bool
		for _, o := range d.Objectives {
			if o.ID != sc.obj.ID {
				continue
			}
			found = true
			if o.Actions != 3 {
				t.Errorf("the digest counts %d actions for objective %s, want the 3 the audit log holds", o.Actions, sc.obj.ID)
			}
		}
		if !found {
			t.Errorf("the digest does not include objective %s: %+v", sc.obj.ID, d.Objectives)
		}
		for _, dec := range d.Decisions {
			if dec.CheckpointID == cpID {
				t.Errorf("the digest still lists checkpoint %s as owed after ada answered it: %+v", cpID, dec)
			}
		}
	})

	// What A6 shows and does not: it scores the verifier-backed remediation
	// criterion only. Root-cause is judged by a model, and no judge is wired
	// into this harness, so "the template reaches its criteria" is shown for
	// the criterion a capability settles.
	t.Run("A6 verify", func(t *testing.T) {
		if len(acted) == 0 {
			t.Fatal("no outcomes to verify: stage A3 did not act")
		}
		sc.obj.SuccessCriteria = remediationCriterion(t, sc.obj.SuccessCriteria)
		if score, met := stepVerify(ctx, sc, acted); !met || score < 1 {
			t.Errorf("remediation criterion scored %.2f (met=%v) after the alert cleared", score, met)
		}

		// The same plan with a command that clears nothing: the alert is
		// still firing, so the criterion must not be met.
		still := &scriptedObservability{
			alerts:  []observability.Alert{firingAlert()},
			markers: map[string]string{incidentAlertID: filepath.Join(t.TempDir(), "never")},
		}
		sc2 := incidentContext(t, still, true)
		outcomes := stepAct(ctx, sc2, incidentPlan("true"))
		if len(outcomes) != 3 {
			t.Fatalf("got %d outcomes, want 3: %+v", len(outcomes), outcomes)
		}
		if o := outcomeOf(t, outcomes, software.CapRunRemediation); !o.Result.Success {
			t.Fatalf("the no-op remediation itself failed: %s", o.Result.Error)
		}
		if o := outcomeOf(t, outcomes, software.CapAlertsResolved); o.Result.Success {
			t.Error("alerts_resolved succeeded while the alert was still firing")
		}
		sc2.obj.SuccessCriteria = remediationCriterion(t, sc2.obj.SuccessCriteria)
		if score, met := stepVerify(ctx, sc2, outcomes); met || score != 0 {
			t.Errorf("remediation criterion scored %.2f (met=%v) though the alert never cleared", score, met)
		}
	})
}

func remediationCriterion(t *testing.T, all []objective.Criterion) []objective.Criterion {
	t.Helper()
	var out []objective.Criterion
	for _, c := range all {
		if c.ID == "remediation" {
			out = append(out, c)
		}
	}
	if len(out) != 1 {
		t.Fatalf("the incident template has %d remediation criteria, want 1", len(out))
	}
	return out
}

// A deployment that binds no observability instance, or binds one that is not
// active, cannot see. Blind is never quiet and never resolved.
//
// B2 (reconcile's sensing lists the environment as blind and produces NO SHA
// for it) is in internal/feature/reconcile/incident_blind_test.go: reconcile's
// sensing function is unexported and cannot be reached from package loop.
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
