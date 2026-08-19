package karakuri

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/conformance"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
)

func TestConformance(t *testing.T) {
	for _, r := range conformance.New().Run(context.Background(), New()) {
		if !r.Passed {
			t.Errorf("%s: %s", r.Check, r.Message)
		}
	}
}

// The pack analyses and drafts; the software pack does the writing, in a
// worktree, through a pull request somebody reviews.
//
// A single pack that could both decide what Karakuri should become and carry
// it out would be one bug away from a system that edits its own bounds, so
// this is asserted rather than left to a comment.
func TestPackOwnsNoWriteCapability(t *testing.T) {
	for _, c := range New().Capabilities() {
		for _, verb := range []string{".act.", ".write", ".push", ".merge", ".delete", ".apply"} {
			if strings.Contains(string(c.ID), verb) {
				t.Errorf("capability %q looks like it changes something; the writing belongs to the software pack", c.ID)
			}
		}
	}
}

// The agent that reasons about what Karakuri should become may never act
// unsupervised, whatever autonomy a standing objective has earned. The
// objective's ceiling bounds how far it may be promoted; this bounds what
// promotion can ever mean here.
func TestMaintainerCannotActUnsupervised(t *testing.T) {
	var found bool
	for _, def := range New().AgentDefinitions() {
		if def.ID != "karakuri-maintainer" {
			continue
		}
		found = true
		if def.Authority.MaxAutonomousActions != 0 {
			t.Errorf("MaxAutonomousActions = %d, want 0", def.Authority.MaxAutonomousActions)
		}
		// Named explicitly, because this assertion was worthless until the
		// decide step was fixed: it read a cap of 0 as "no cap", so the field
		// this test checks was doing nothing at all. What 0 *means* is pinned
		// by TestZeroAutonomousActionsEscalatesRatherThanActing in the loop
		// package; this only has to make sure nobody writes the opt-out here.
		if def.Authority.MaxAutonomousActions == coreagent.UnlimitedActions {
			t.Error("the maintainer opted out of the autonomous-action cap")
		}
		if def.Authority.ConfidenceThreshold < 1.0 {
			t.Errorf("ConfidenceThreshold = %v, want at least 1.0", def.Authority.ConfidenceThreshold)
		}
		if def.Authority.CanModifyObjective {
			t.Error("the agent deciding what Karakuri should do can edit what it was asked to do")
		}
		if def.Authority.CanDelegate {
			t.Error("the maintainer can delegate, which routes around its own bounds")
		}
	}
	if !found {
		t.Fatal("karakuri-maintainer is not defined")
	}
}

// Both environments observe. Neither changes anything, and both say so rather
// than succeeding quietly — the value of letting Karakuri watch itself is that
// the watching cannot be edited by the thing being watched.
func TestEnvironmentsRefuseToAct(t *testing.T) {
	ctx := context.Background()
	for _, f := range New().EnvironmentFactories() {
		env, err := f.Build(environment.BuildContext{})
		if err != nil {
			t.Fatalf("build %s: %v", f.EnvID, err)
		}
		res, err := env.Act(ctx, environment.Action{CapabilityID: "software.act.write_code"})
		if err != nil {
			t.Fatalf("act on %s returned an error rather than a refusal: %v", f.EnvID, err)
		}
		if res.Success {
			t.Errorf("%s accepted an action", f.EnvID)
		}
		if res.Error == "" {
			t.Errorf("%s refused silently; a refusal nobody can read is a silent success", f.EnvID)
		}
	}
}

// A deployment with no telemetry wired says so, rather than reporting zeroes
// that read as a perfectly healthy system.
func TestUnwiredTelemetryReportsItself(t *testing.T) {
	env := &telemetryEnv{}
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.State["available"] != false {
		t.Error("an unwired telemetry environment did not report itself unavailable")
	}
	if obs.Version != "" {
		t.Error("an unwired environment produced a fingerprint, which would read as a still world")
	}

	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.SHA != "" {
		t.Error("an unwired environment produced a SHA; the supervisor must read it as blind")
	}
}

// The fingerprint hashes the shape of a deployment, not its counters.
//
// A hash over raw numbers would move every time anything happened, and a
// self-improvement objective reconciling on every counter tick would spend all
// day noticing that work occurred.
func TestFingerprintIgnoresOrdinaryChurn(t *testing.T) {
	base := coretelemetry.Snapshot{
		Work:       coretelemetry.WorkStats{Senses: 400, Reconciles: 12, Failures: 4},
		Escalation: coretelemetry.EscalationStats{Approvals: 9, Rejections: 1, Pending: 2},
	}

	busier := base
	busier.Work.Senses = 900
	busier.Work.Reconciles = 30
	busier.Escalation.Approvals = 40
	if coarseFingerprint(base) != coarseFingerprint(busier) {
		t.Error("a busy week moved the fingerprint; ordinary churn is not news")
	}

	// A new bottleneck is news.
	worse := base
	worse.Bottlenecks = []coretelemetry.Bottleneck{{
		Kind: coretelemetry.BottleneckStaleDecision, Detail: "waiting more than a day", Count: 3,
	}}
	if coarseFingerprint(base) == coarseFingerprint(worse) {
		t.Error("a new bottleneck did not move the fingerprint")
	}

	// So is a decision queue growing by an order of magnitude.
	piling := base
	piling.Escalation.Pending = 40
	if coarseFingerprint(base) == coarseFingerprint(piling) {
		t.Error("a decision queue growing from 2 to 40 did not move the fingerprint")
	}

	// And so is an objective being taken out of rotation.
	blocked := base
	blocked.Objectives.Blocked = 1
	if coarseFingerprint(base) == coarseFingerprint(blocked) {
		t.Error("a blocked objective did not move the fingerprint")
	}

	// The bottleneck set is hashed order-independently, so an implementation
	// that returned it in a different order would not fire spuriously.
	a := base
	a.Bottlenecks = []coretelemetry.Bottleneck{{Kind: "x", Detail: "one", Count: 1}, {Kind: "y", Detail: "two", Count: 1}}
	b := base
	b.Bottlenecks = []coretelemetry.Bottleneck{{Kind: "y", Detail: "two", Count: 1}, {Kind: "x", Detail: "one", Count: 1}}
	if coarseFingerprint(a) != coarseFingerprint(b) {
		t.Error("bottleneck ordering changed the fingerprint")
	}
}

// "Nobody rejected anything" and "nobody decided anything" are opposite
// signals, and a system reasoning about its own trustworthiness must not
// confuse them.
func TestApprovalRateDistinguishesUndecided(t *testing.T) {
	none := coretelemetry.EscalationStats{Escalations: 5}
	if got := none.ApprovalRate(); got != -1 {
		t.Errorf("approval rate with nothing resolved = %v, want -1", got)
	}
	if rateBand(none.ApprovalRate()) != "undecided" {
		t.Error("an undecided queue banded as an approval rate")
	}

	allRejected := coretelemetry.EscalationStats{Rejections: 4}
	if got := allRejected.ApprovalRate(); got != 0 {
		t.Errorf("approval rate with everything rejected = %v, want 0", got)
	}
	if rateBand(0) == rateBand(-1) {
		t.Error("everything-rejected and nothing-decided band the same")
	}
}

// The repository environment sorts before hashing, for the same reason the
// reconcile fingerprint sorts environment IDs: an adapter free to return rows
// in any order would otherwise report drift on nothing having changed.
func TestRepoEnvironmentWithoutAdapter(t *testing.T) {
	env := &repoEnv{}
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.State["available"] != false {
		t.Error("an unbound repository environment did not report itself unavailable")
	}
	if obs.Timestamp.After(time.Now().UTC().Add(time.Minute)) {
		t.Error("observation timestamp is in the future")
	}
}

// The self-improvement objective is cross-domain by construction: this pack
// cannot mark its own homework on the part it does not do.
func TestSelfImproveVerifiesThroughTheSoftwarePack(t *testing.T) {
	for _, tpl := range New().ObjectiveTemplates() {
		if tpl.ID != "karakuri.objective.self_improve" {
			continue
		}
		var foreign bool
		for _, c := range tpl.SuccessCriteria {
			if strings.HasPrefix(string(c.Verifier), "software.") {
				foreign = true
			}
		}
		if !foreign {
			t.Error("self-improvement is verified entirely by the pack that proposes it")
		}
		return
	}
	t.Fatal("karakuri.objective.self_improve is not defined")
}
