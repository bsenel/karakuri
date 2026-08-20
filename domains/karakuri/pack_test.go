package karakuri

import (
	"context"
	"strings"
	"testing"
	"time"

	software "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/conformance"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
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

// Neither environment changes anything, and both say so rather than
// succeeding quietly — the value of letting Karakuri watch itself is that the
// watching cannot be edited by the thing being watched.
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

// The other half, and the half that was missing: refusing everything is not
// the same as refusing writes, and the test above cannot tell them apart —
// it only ever offers a foreign capability, which both a correct environment
// and a wholly inert one decline.
//
// Both environments did refuse everything, so the pack could not execute the
// three capabilities it exists to provide. Enabling it and running
// self_improve produced six failed actions and no analysis at all.
func TestEnvironmentsExecuteThePackOwnCapabilities(t *testing.T) {
	ctx := context.Background()
	envs := map[environment.EnvironmentID]environment.Environment{}
	for _, f := range New().EnvironmentFactories() {
		env, err := f.Build(environment.BuildContext{Telemetry: stubReader{}})
		if err != nil {
			t.Fatalf("build %s: %v", f.EnvID, err)
		}
		envs[f.EnvID] = env
	}

	// Params that satisfy each capability's declared Required set. Passing
	// something that satisfies none of them would assert that invalid input
	// succeeds, which is not the property under test.
	valid := map[capability.CapabilityID]map[string]any{
		CapAnalyseUsage:   {},
		CapProposeRoadmap: {"problem": "sensing costs nothing and nobody reads the result"},
		CapDraftADR:       {"decision": "the supervisor is a caller, not a second policy gate"},
	}

	// Driven from the pack's own declarations rather than a hardcoded list, so
	// a capability added later without a route fails here.
	for _, c := range New().Capabilities() {
		envID, routed := servedBy[c.ID]
		if !routed {
			t.Errorf("capability %q is declared but no environment serves it; it would be refused at runtime", c.ID)
			continue
		}
		res, err := envs[envID].Act(ctx, environment.Action{CapabilityID: c.ID, Params: valid[c.ID]})
		if err != nil {
			t.Fatalf("act %s on %s: %v", c.ID, envID, err)
		}
		if !res.Success {
			t.Errorf("%s refused %s, a capability this pack declares: %s", envID, c.ID, res.Error)
		}
	}
}

// The drafting capabilities declare a required input. A capability that
// returns success for empty input feeds a perfect success rate into
// procedural memory, which biases the next plan's confidence upward for
// having produced nothing.
func TestDraftingRefusesEmptyInput(t *testing.T) {
	env := &repoEnv{}
	for _, tc := range []struct {
		cap     capability.CapabilityID
		missing string
	}{
		{CapProposeRoadmap, "problem"},
		{CapDraftADR, "decision"},
	} {
		for _, params := range []map[string]any{nil, {}, {tc.missing: "   "}} {
			res, err := env.Act(context.Background(), environment.Action{CapabilityID: tc.cap, Params: params})
			if err != nil {
				t.Fatalf("act %s: %v", tc.cap, err)
			}
			if res.Success {
				t.Errorf("%s reported a draft with no %q in it", tc.cap, tc.missing)
			}
		}
	}
}

// The recorded draft must not alias the caller's map: stepAct writes into
// params for some capabilities and persists it beside this result, so an alias
// would let a later write retroactively edit what was recorded.
func TestRecordedDraftIsCopied(t *testing.T) {
	params := map[string]any{"problem": "original"}
	res, err := (&repoEnv{}).Act(context.Background(),
		environment.Action{CapabilityID: CapProposeRoadmap, Params: params})
	if err != nil || !res.Success {
		t.Fatalf("act: %v %s", err, res.Error)
	}
	params["problem"] = "mutated after the fact"
	if got := res.StateDelta["draft"].(map[string]any)["problem"]; got != "original" {
		t.Errorf("recorded draft followed a later mutation: %v", got)
	}
}

// "Sufficient" has to be able to be both. The earlier version ORed in an
// objective count the reader never windows, so it was true in every real
// deployment — including one whose entire window was empty.
func TestSufficientDistinguishesAnEmptyWindow(t *testing.T) {
	// The production shape that the degenerate version got wrong: objectives
	// exist, and nothing at all happened in the window.
	quiet := coretelemetry.Snapshot{Objectives: coretelemetry.ObjectiveStats{Total: 7, Standing: 3}}
	if sufficient(quiet) {
		t.Error("a window with no work, no escalations and no bottlenecks reported sufficient evidence")
	}

	for name, snap := range map[string]coretelemetry.Snapshot{
		"senses":      {Work: coretelemetry.WorkStats{Senses: 1}},
		"reconciles":  {Work: coretelemetry.WorkStats{Reconciles: 1}},
		"actions":     {Work: coretelemetry.WorkStats{Actions: 1}},
		"escalations": {Escalation: coretelemetry.EscalationStats{Escalations: 1}},
		"bottleneck":  {Bottlenecks: []coretelemetry.Bottleneck{{Kind: "x", Count: 1}}},
	} {
		if !sufficient(snap) {
			t.Errorf("a window containing %s reported no evidence", name)
		}
	}
}

// The insufficiency marker has to reach the path that plans. stepObserve's
// result is what stepReason reasons from; a marker only on the Act result
// informs nothing, because Act runs after the decision it would have changed.
func TestObserveCarriesTheEvidenceMarker(t *testing.T) {
	obs, err := (&telemetryEnv{reader: stubReader{}}).Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.State["sufficient"] != false {
		t.Error("Observe reported an empty deployment without saying its evidence was insufficient")
	}
}

// A deployment that enabled the pack without wiring the reader has a
// configuration gap, not a broken capability. Recording it as a failed action
// would raise a failing_capability bottleneck against the pack's own core
// capability and bias its procedural confidence down.
func TestUnwiredReaderIsNotACapabilityFailure(t *testing.T) {
	res, err := (&telemetryEnv{}).Act(context.Background(), environment.Action{CapabilityID: CapAnalyseUsage})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Error("an unwired reader was recorded as the capability failing")
	}
	if res.StateDelta["available"] != false || res.StateDelta["sufficient"] != false {
		t.Error("an unwired reader did not report itself unavailable and without evidence")
	}
}

// The environment's twin is a ceiling. A plan asking for the whole deployment
// from inside one tenant is asking for the cross-tenant read the telemetry
// reader was fixed to prevent.
func TestAnalyseUsageCannotWidenBeyondItsTwin(t *testing.T) {
	spy := &recordingReader{}
	env := &telemetryEnv{reader: spy, twinID: "twin-a"}
	_, err := env.Act(context.Background(), environment.Action{
		CapabilityID: CapAnalyseUsage,
		Params:       map[string]any{"twin_id": "", "window": "24h"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if spy.last.TwinID != "twin-a" {
		t.Errorf("query twin = %q, want twin-a: a plan widened its own scope", spy.last.TwinID)
	}
	// The window is the caller's to choose; only the twin is bounded.
	if got := spy.last.Since; time.Since(got) > 25*time.Hour {
		t.Errorf("declared window of 24h was ignored (since=%v)", got)
	}
}

// A cross-domain criterion has to name a capability the pack it points at
// actually exports.
//
// self_improve named software.act.open_pull_request, which nothing declares —
// the software pack calls it software.act.create_pr. The criterion carrying
// the most weight in the template could never be satisfied. Nothing caught it
// because the conformance suite deliberately does not resolve foreign domains
// (correct per ADR 017: a pack is valid on its own), which leaves exactly this
// unchecked. Phase 24 puts the general check on the registry at boot; this
// pins the one cross-pack reference this pack actually makes.
func TestForeignVerifiersResolveInTheDomainTheyName(t *testing.T) {
	exported := map[string]map[capability.CapabilityID]bool{
		"software": {},
	}
	for _, c := range software.New().Capabilities() {
		exported["software"][c.ID] = true
	}

	var checked int
	for _, tpl := range New().ObjectiveTemplates() {
		for _, crit := range tpl.SuccessCriteria {
			if crit.Domain == "" || crit.Domain == "karakuri" {
				continue
			}
			owned, known := exported[crit.Domain]
			if !known {
				t.Fatalf("%s: criterion %q names domain %q, which this test does not know how to resolve",
					tpl.ID, crit.ID, crit.Domain)
			}
			checked++
			if !owned[crit.Verifier] {
				t.Errorf("%s: criterion %q is verified by %q, which the %s pack does not export",
					tpl.ID, crit.ID, crit.Verifier, crit.Domain)
			}
		}
	}
	if checked == 0 {
		t.Error("no cross-domain criterion was checked; this test has stopped testing anything")
	}
}

// stubReader is an empty deployment: wired, and with nothing to report.
type stubReader struct{}

func (stubReader) Snapshot(context.Context, coretelemetry.Query) (coretelemetry.Snapshot, error) {
	return coretelemetry.Snapshot{}, nil
}

// recordingReader captures the query it was asked for.
type recordingReader struct{ last coretelemetry.Query }

func (r *recordingReader) Snapshot(_ context.Context, q coretelemetry.Query) (coretelemetry.Snapshot, error) {
	r.last = q
	return coretelemetry.Snapshot{}, nil
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
