package software

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

const (
	sreAgentID         = "software.agent.sre"
	incidentTemplateID = "software.objective.incident_response"
)

func sreAgent(t *testing.T) agent.Definition {
	t.Helper()
	for _, def := range New().AgentDefinitions() {
		if def.ID == sreAgentID {
			return def
		}
	}
	t.Fatalf("the software pack declares no agent %q", sreAgentID)
	return agent.Definition{}
}

func incidentTemplate(t *testing.T) objective.Template {
	t.Helper()
	for _, tmpl := range New().ObjectiveTemplates() {
		if tmpl.ID == incidentTemplateID {
			return tmpl
		}
	}
	t.Fatalf("the software pack declares no template %q", incidentTemplateID)
	return objective.Template{}
}

// A remediation changes a running system, so the SRE asks first — however
// confident the plan and however much room the action cap leaves.
//
// This runs the SRE's real bounds through the policy rather than reading the
// field back: one action is well under the cap and 0.99 is well over the
// threshold, so the only thing left to escalate is the approval list.
func TestSRERemediationEscalatesHoweverConfident(t *testing.T) {
	b := sreAgent(t).Authority

	if b.MaxAutonomousActions < 1 && b.MaxAutonomousActions != agent.UnlimitedActions {
		t.Fatalf("the SRE's action cap is %d: a one-action plan would escalate for the cap and this test would prove nothing",
			b.MaxAutonomousActions)
	}
	if b.ConfidenceThreshold >= 0.99 {
		t.Fatalf("the SRE's confidence threshold is %.2f: a 0.99 plan would escalate for confidence and this test would prove nothing",
			b.ConfidenceThreshold)
	}

	v := b.Decide(0.99, b.ConfidenceThreshold, []capability.CapabilityID{CapRunRemediation}, agent.Evidence{})
	if !v.Escalate {
		t.Fatalf("the SRE planned %s at confidence 0.99 and ran it without asking", CapRunRemediation)
	}
	if !strings.Contains(v.Reason, CapRunRemediation) || !strings.Contains(v.Reason, "requires approval") {
		t.Errorf("escalation reason = %q, want it to name %s as requiring approval", v.Reason, CapRunRemediation)
	}
}

// Looking is not acting. A plan that only reads logs and metrics and checks
// whether the alerts cleared runs without a checkpoint.
func TestSREInvestigationDoesNotNeedApproval(t *testing.T) {
	b := sreAgent(t).Authority

	plan := []capability.CapabilityID{CapFetchLogs, CapFetchMetrics, CapAlertsResolved}
	v := b.Decide(0.99, b.ConfidenceThreshold, plan, agent.Evidence{})
	if strings.Contains(v.Reason, "requires approval") {
		t.Errorf("a plan that only observes and verifies escalated for approval: %s", v.Reason)
	}
	if v.Escalate {
		t.Errorf("a plan that only observes and verifies escalated: %s", v.Reason)
	}
	if v.Allowed != len(plan) {
		t.Errorf("Allowed = %d, want all %d actions", v.Allowed, len(plan))
	}
}

// The SRE can see the incident, change the system, and check that the change
// worked — and the two capabilities that reach outside it are the two it asks
// about.
func TestSREAgentCanRemediateAndAsksFirst(t *testing.T) {
	sre := sreAgent(t)

	has := map[capability.CapabilityID]bool{}
	for _, c := range sre.Capabilities {
		has[c] = true
	}
	for _, want := range []capability.CapabilityID{
		CapFetchLogs,
		CapFetchMetrics,
		"software.act.write_code",
		"software.verify.run_tests",
		CapRunRemediation,
		CapAlertsResolved,
	} {
		if !has[want] {
			t.Errorf("the SRE agent lacks %q", want)
		}
	}

	wantApproval := map[capability.CapabilityID]bool{
		"software.act.create_pr": true,
		CapRunRemediation:        true,
	}
	if len(sre.Authority.RequiresApprovalFor) != len(wantApproval) {
		t.Errorf("RequiresApprovalFor = %v, want exactly software.act.create_pr and %s",
			sre.Authority.RequiresApprovalFor, CapRunRemediation)
	}
	seen := map[capability.CapabilityID]bool{}
	for _, c := range sre.Authority.RequiresApprovalFor {
		if !wantApproval[c] {
			t.Errorf("RequiresApprovalFor names %q, which the SRE should not be gated on", c)
		}
		seen[c] = true
	}
	for c := range wantApproval {
		if !seen[c] {
			t.Errorf("RequiresApprovalFor does not name %q", c)
		}
	}
}

// "Remediation applied" is settled by the alerts being gone, not by a test
// suite passing: a green build says nothing about whether production recovered.
func TestIncidentRemediationIsVerifiedByAlertsResolved(t *testing.T) {
	tmpl := incidentTemplate(t)

	var remediation *objective.Criterion
	for i := range tmpl.SuccessCriteria {
		if tmpl.SuccessCriteria[i].ID == "remediation" {
			remediation = &tmpl.SuccessCriteria[i]
		}
	}
	if remediation == nil {
		t.Fatalf("%s has no criterion %q", incidentTemplateID, "remediation")
	}
	if remediation.Verifier != CapAlertsResolved {
		t.Errorf("remediation verifier = %q, want %s", remediation.Verifier, CapAlertsResolved)
	}
	if remediation.Weight != 0.6 {
		t.Errorf("remediation weight = %v, want 0.6", remediation.Weight)
	}

	pack := New()
	declared := false
	for _, c := range pack.Capabilities() {
		if c.ID == remediation.Verifier {
			declared = true
		}
	}
	if !declared {
		t.Errorf("remediation is verified by %q, which this pack does not declare", remediation.Verifier)
	}
	served := false
	for _, f := range pack.EnvironmentFactories() {
		for _, c := range f.Serves {
			if c == remediation.Verifier {
				served = true
			}
		}
	}
	if !served {
		t.Errorf("remediation is verified by %q, which no environment serves: the criterion falls back to a judgement call",
			remediation.Verifier)
	}
}

// An incident is the SRE's to run, and it stays the one software template
// where every action waits for a human.
func TestIncidentResponseNamesTheSREAndStaysHighRisk(t *testing.T) {
	tmpl := incidentTemplate(t)

	if len(tmpl.SuggestedAgents) != 1 {
		t.Fatalf("SuggestedAgents has %d definitions, want exactly one (%s)", len(tmpl.SuggestedAgents), sreAgentID)
	}
	if got := tmpl.SuggestedAgents[0].ID; got != sreAgentID {
		t.Errorf("SuggestedAgents[0].ID = %q, want %q", got, sreAgentID)
	}

	if tmpl.Risk != objective.RiskHigh {
		t.Errorf("Risk = %v, want objective.RiskHigh", tmpl.Risk)
	}

	found := false
	for _, c := range tmpl.Constraints {
		if c.ID == "approval-required" {
			found = true
			if !c.Hard {
				t.Errorf("constraint %q is no longer hard", c.ID)
			}
		}
	}
	if !found {
		t.Errorf("%s lost its approval-required constraint", incidentTemplateID)
	}
}
