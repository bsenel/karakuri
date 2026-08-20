package software

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
)

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

// The assertion that replaces the pack boundary.
//
// The karakuri pack was credited with the guarantee "the thing that decides
// what to change cannot change anything", enforced by the pack owning no
// write capability. That was never what enforced it: a pack is a namespace,
// and stepAct resolves an action's environment across every domain an
// objective names — which is what made cross-domain objectives work at all.
//
// The agent's own capability list is where the property actually lives, and
// it survives the two sets living in one pack.
// It guards the agent that actually runs, which it did not always: selection
// took the first agent a domain declared, so self_improve ran under the
// strategist in a nine-agent pack and this test guarded someone else.
// Template.SuggestedAgents now reaches the objective and selection honours it,
// pinned by TestSelfImproveTemplatesNameTheirAgent below and by
// TestSelectAgentHonoursTheObjectiveAgent in the loop package.
func TestMaintainerHoldsNoMutatingCapability(t *testing.T) {
	var found bool
	for _, def := range New().AgentDefinitions() {
		if def.ID != "software.agent.maintainer" {
			continue
		}
		found = true
		held := map[capability.CapabilityID]bool{}
		for _, c := range def.Capabilities {
			held[c] = true
		}
		for _, mutating := range mutatingCapabilities() {
			if held[mutating] {
				t.Errorf("the maintainer holds %q, which changes the repository", mutating)
			}
		}
		if def.Authority.MaxAutonomousActions != 0 {
			t.Errorf("MaxAutonomousActions = %d, want 0", def.Authority.MaxAutonomousActions)
		}
		if def.Authority.MaxAutonomousActions == coreagent.UnlimitedActions {
			t.Error("the maintainer opted out of the autonomous-action cap")
		}
		if def.Authority.ConfidenceThreshold < 1.0 {
			t.Errorf("ConfidenceThreshold = %v, want at least 1.0", def.Authority.ConfidenceThreshold)
		}
		if def.Authority.CanModifyObjective || def.Authority.CanDelegate {
			t.Error("the maintainer can edit its objective or delegate around its bounds")
		}
	}
	if !found {
		t.Fatal("software.agent.maintainer is not defined")
	}
}

// The mutating set has to name capabilities that exist, or the test above
// passes by checking nothing.
func TestMutatingCapabilitiesAreReal(t *testing.T) {
	declared := map[capability.CapabilityID]bool{}
	for _, c := range New().Capabilities() {
		declared[c.ID] = true
	}
	for _, m := range mutatingCapabilities() {
		if !declared[m] {
			t.Errorf("%q is listed as mutating but this pack does not declare it", m)
		}
	}
}

// Every self-improvement capability is routed to an environment that runs it.
func TestSelfImproveCapabilitiesAreRouted(t *testing.T) {
	ctx := context.Background()
	valid := map[capability.CapabilityID]map[string]any{
		CapAnalyseUsage:   {},
		CapProposeRoadmap: {"problem": "sensing costs nothing and nobody reads the result"},
		CapDraftADR:       {"decision": "the supervisor is a caller, not a second policy gate"},
	}
	envs := map[environment.EnvironmentID]environment.Environment{}
	for _, f := range New().EnvironmentFactories() {
		env, err := f.Build(environment.BuildContext{Telemetry: stubReader{}})
		if err != nil {
			continue // a factory that needs wiring this test does not supply
		}
		envs[f.EnvID] = env
	}

	for _, c := range selfImproveCapabilities() {
		envID, routed := servedBy[c.ID]
		if !routed {
			t.Errorf("capability %q is declared but no environment serves it", c.ID)
			continue
		}
		env, built := envs[envID]
		if !built {
			t.Fatalf("environment %q did not build", envID)
		}
		res, err := env.Act(ctx, environment.Action{CapabilityID: c.ID, Params: valid[c.ID]})
		if err != nil {
			t.Fatalf("act %s on %s: %v", c.ID, envID, err)
		}
		if !res.Success {
			t.Errorf("%s refused %s: %s", envID, c.ID, res.Error)
		}
	}
}

// Drafting must not depend on a version-control adapter. It touches no
// repository, and routing it behind the adapter check would report a draft
// that never happened on any deployment without git wired.
func TestDraftingWorksWithoutAnAdapter(t *testing.T) {
	env := &gitEnv{id: EnvGit}
	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: CapDraftADR,
		Params:       map[string]any{"decision": "a pack is a namespace, not a boundary"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Errorf("drafting refused with no adapter wired: %s", res.Error)
	}
}

// A capability that returns success for empty input feeds a perfect success
// rate into procedural memory, biasing the next plan's confidence up for
// having produced nothing.
func TestDraftingRefusesEmptyInput(t *testing.T) {
	env := &gitEnv{id: EnvGit}
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

// The wired reader is the gate, and it gates by answering honestly rather
// than by refusing to exist.
//
// An earlier version had the factory error without a reader. Conformance
// rejected it — a declared factory must be constructible — and it was right:
// every other adapter-backed environment here builds and degrades. What the
// property needs is that an unwired deployment learns nothing about the
// platform, and it learns nothing because there is nothing behind the port.
func TestPlatformTelemetryGatesOnItsReader(t *testing.T) {
	env, err := platformTelemetryFactory().Build(environment.BuildContext{})
	if err != nil {
		t.Fatalf("the telemetry environment must be constructible: %v", err)
	}

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.State["available"] != false {
		t.Error("an unwired telemetry environment did not report itself unavailable")
	}
	if obs.Version != "" {
		t.Error("an unwired environment produced a fingerprint, which reads as a still world")
	}

	res, err := env.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseUsage})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	// Not a capability failure: recorded as one, three of them would raise a
	// failing_capability bottleneck against the pack's own analysis and bias
	// its procedural confidence down for a configuration gap.
	if !res.Success {
		t.Error("an unwired reader was recorded as the capability failing")
	}
	if res.StateDelta["available"] != false || res.StateDelta["sufficient"] != false {
		t.Error("an unwired reader did not report itself unavailable and without evidence")
	}
}

// "Sufficient" has to be able to be both.
func TestSufficientDistinguishesAnEmptyWindow(t *testing.T) {
	quiet := coretelemetry.Snapshot{Objectives: coretelemetry.ObjectiveStats{Total: 7, Standing: 3}}
	if sufficient(quiet) {
		t.Error("a window with no work, no escalations and no bottlenecks reported sufficient evidence")
	}
	busy := coretelemetry.Snapshot{Work: coretelemetry.WorkStats{Reconciles: 1}}
	if !sufficient(busy) {
		t.Error("a window containing a reconcile reported no evidence")
	}
}

// The environment's twin is a ceiling. A plan asking for the whole deployment
// from inside one tenant is asking for a cross-tenant read.
func TestAnalyseUsageCannotWidenBeyondItsTwin(t *testing.T) {
	spy := &recordingReader{}
	env := &telemetryEnv{reader: spy, twinID: "twin-a"}
	if _, err := env.Act(context.Background(), environment.Action{
		CapabilityID: CapAnalyseUsage,
		Params:       map[string]any{"twin_id": "", "window": "24h"},
	}); err != nil {
		t.Fatalf("act: %v", err)
	}
	if spy.last.TwinID != "twin-a" {
		t.Errorf("query twin = %q, want twin-a: a plan widened its own scope", spy.last.TwinID)
	}
}

// The fingerprint hashes the shape of a deployment, not its counters.
func TestFingerprintIgnoresOrdinaryChurn(t *testing.T) {
	base := coretelemetry.Snapshot{
		Work:       coretelemetry.WorkStats{Senses: 400, Reconciles: 12, Failures: 4},
		Escalation: coretelemetry.EscalationStats{Approvals: 9, Rejections: 1, Pending: 2},
	}
	busier := base
	busier.Work.Senses = 900
	busier.Escalation.Approvals = 40
	if coarseFingerprint(base) != coarseFingerprint(busier) {
		t.Error("a busy week moved the fingerprint; ordinary churn is not news")
	}
	worse := base
	worse.Bottlenecks = []coretelemetry.Bottleneck{{Kind: coretelemetry.BottleneckStaleDecision, Count: 3}}
	if coarseFingerprint(base) == coarseFingerprint(worse) {
		t.Error("a new bottleneck did not move the fingerprint")
	}
}

// self_improve is verified in part by a capability this pack exports. It
// previously named software.act.open_pull_request across a domain boundary,
// which nothing resolved.
func TestSelfImproveVerifiersResolve(t *testing.T) {
	declared := map[capability.CapabilityID]bool{}
	for _, c := range New().Capabilities() {
		declared[c.ID] = true
	}
	var checked int
	for _, tpl := range New().ObjectiveTemplates() {
		if tpl.ID != "software.objective.self_improve" {
			continue
		}
		for _, crit := range tpl.SuccessCriteria {
			checked++
			if crit.Domain != "" {
				t.Errorf("criterion %q is cross-domain again; that is what hid the dangling verifier", crit.ID)
			}
			if !declared[crit.Verifier] {
				t.Errorf("criterion %q is verified by %q, which this pack does not export", crit.ID, crit.Verifier)
			}
		}
	}
	if checked == 0 {
		t.Fatal("software.objective.self_improve is not defined")
	}
}

// The default agent for a software objective must escalate too, because it is
// the one that actually runs a self_improve objective today.
//
// Written after a live run showed the strategist handling it and escalating on
// its confidence threshold rather than the maintainer's bounds. If somebody
// reorders the pack's agents or loosens the first one's cap, self-improvement
// silently starts acting unsupervised, and the maintainer test above would not
// notice.
func TestTheDefaultSoftwareAgentAlsoEscalates(t *testing.T) {
	defs := New().AgentDefinitions()
	if len(defs) == 0 {
		t.Fatal("the software pack declares no agents")
	}
	first := defs[0]
	if first.Authority.MaxAutonomousActions != 0 {
		t.Errorf("the default agent %q has MaxAutonomousActions = %d; a self_improve objective "+
			"runs under it and would act without asking",
			first.ID, first.Authority.MaxAutonomousActions)
	}
}

// ── Phase 26: the write path ─────────────────────────────────────────────

// The capabilities that write declare that they need a workspace.
//
// This was decided by matching the capability's name against ".write_code"
// and ".write_test", which gave a worktree to two capabilities with no
// implementation and withheld one from delegate_to_cli — the only one that
// could actually write. A capability that needs a workspace is something only
// the capability knows.
func TestWritingCapabilitiesDeclareTheirWorkspace(t *testing.T) {
	need := map[capability.CapabilityID]bool{
		"software.act.write_code":      true,
		"software.act.write_test":      true,
		"software.act.delegate_to_cli": true,
		"software.act.create_pr":       true,
	}
	seen := map[capability.CapabilityID]bool{}
	for _, c := range New().Capabilities() {
		seen[c.ID] = true
		if need[c.ID] && !c.NeedsWorkspace {
			t.Errorf("%q writes files but does not declare NeedsWorkspace", c.ID)
		}
		if !need[c.ID] && c.NeedsWorkspace {
			t.Errorf("%q declares NeedsWorkspace but does not write", c.ID)
		}
	}
	for id := range need {
		if !seen[id] {
			t.Errorf("%q is expected to write but this pack does not declare it", id)
		}
	}
}

// write_code and write_test are delegated to the coding-agent CLI rather than
// left declared and unimplemented. They were stubs for three phases: the loop
// provisioned a worktree for them and then routed them to noopEnv.
func TestWriteCapabilitiesReachTheCLI(t *testing.T) {
	for _, capID := range []string{
		"software.act.write_code",
		"software.act.write_test",
		"software.act.delegate_to_cli",
	} {
		if _, served := cliPrompt(environment.Action{
			CapabilityID: capability.CapabilityID(capID),
			Params:       map[string]any{"prompt": "add a test"},
		}); !served {
			t.Errorf("%q is not served by the CLI environment; it would reach noopEnv", capID)
		}
	}
	// And nothing else is swept in.
	if _, served := cliPrompt(environment.Action{CapabilityID: "software.act.create_pr"}); served {
		t.Error("create_pr was routed to the CLI delegate")
	}
}

// write_test says so in the prompt. The CLI receives text, and the
// distinction from write_code is not implied by an ID it never sees.
func TestWriteTestAsksForTestsOnly(t *testing.T) {
	got, _ := cliPrompt(environment.Action{
		CapabilityID: "software.act.write_test",
		Params:       map[string]any{"prompt": "cover the budget deferral"},
	})
	if !strings.Contains(strings.ToLower(got), "tests only") {
		t.Errorf("write_test prompt does not distinguish itself from write_code: %q", got)
	}
}

// Arriving with no worktree means provisioning failed. Writing into the
// checked-out tree instead is the one outcome a planner hint explicitly
// forbids, so it refuses rather than guessing a path.
func TestWritingRefusesWithoutAWorktree(t *testing.T) {
	env := &cliEnv{id: "software.env.cli_agent", cli: activeStubCLI{}}
	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: "software.act.write_code",
		Params:       map[string]any{"prompt": "do the thing"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("wrote code with no worktree provisioned")
	}
	if !strings.Contains(res.Error, "worktree") {
		t.Errorf("refusal does not say why: %q", res.Error)
	}
}

// And an empty task is refused rather than delegated, so a plan that names
// the capability and forgets the prompt does not bill a CLI run to say
// nothing.
func TestWritingRefusesAnEmptyTask(t *testing.T) {
	env := &cliEnv{id: "software.env.cli_agent", cli: activeStubCLI{}}
	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: "software.act.write_code",
		Params:       map[string]any{"worktree_path": "/tmp/wt"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("delegated an empty task to the CLI")
	}
}

// activeStubCLI is a wired coding-agent adapter that records nothing.
type activeStubCLI struct{}

func (activeStubCLI) Name() string { return "stub" }
func (activeStubCLI) Active() bool { return true }
func (activeStubCLI) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	return cliagent.DelegateOutput{Summary: "did " + in.Prompt}, nil
}
func (activeStubCLI) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

// The templates name the agent whose bounds their safety story rests on.
//
// Without this the objective inherits "whichever agent this pack declares
// first", which is an arbitrary answer that was right for a two-agent pack and
// wrong the moment self-improvement moved into a nine-agent one.
func TestSelfImproveTemplatesNameTheirAgent(t *testing.T) {
	want := map[string]string{
		"software.objective.self_improve":          "software.agent.maintainer",
		"software.objective.watch_platform_health": "software.agent.analyst",
	}
	declared := map[string]bool{}
	for _, a := range New().AgentDefinitions() {
		declared[string(a.ID)] = true
	}

	var checked int
	for _, tpl := range New().ObjectiveTemplates() {
		expect, ok := want[tpl.ID]
		if !ok {
			continue
		}
		checked++
		if len(tpl.SuggestedAgents) == 0 {
			t.Errorf("%s names no agent; it would run under whichever this pack declares first", tpl.ID)
			continue
		}
		got := string(tpl.SuggestedAgents[0].ID)
		if got != expect {
			t.Errorf("%s names %q, want %q", tpl.ID, got, expect)
		}
		if !declared[got] {
			t.Errorf("%s names agent %q, which this pack does not declare", tpl.ID, got)
		}
	}
	if checked != len(want) {
		t.Errorf("checked %d templates, want %d", checked, len(want))
	}
}
