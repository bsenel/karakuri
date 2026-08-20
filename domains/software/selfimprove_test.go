package software

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
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

// A deployment with no telemetry reader gets no telemetry environment, rather
// than one that answers "unavailable" to everything. This is the gating that
// replaces the karakuri pack's config flag.
func TestPlatformTelemetryNeedsAReader(t *testing.T) {
	if _, err := platformTelemetryFactory().Build(environment.BuildContext{}); err == nil {
		t.Error("the telemetry environment built with no reader wired")
	}
	if _, err := platformTelemetryFactory().Build(environment.BuildContext{Telemetry: stubReader{}}); err != nil {
		t.Errorf("the telemetry environment refused to build with a reader wired: %v", err)
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
