package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

// Phase 39 step 3, the last clause of its acceptance: a criterion is verified
// through the pack's tool and the verdict is still computed in the loop (ADR
// 028 Decision 4 and section b). The plan runs the verifier as an action; the
// real gateway routes it to the pack's server with the call contract; the
// pack returns a result, and stepVerify, not the pack, decides the criterion.

const packVerifier = "software.verify.pack_check"

// verifierServer is a pack's server serving one verifier. It records every
// call it receives and answers with a fixed result.
type verifierServer struct {
	result environment.ActionResult

	mu    sync.Mutex
	calls []verifierCall
}

type verifierCall struct {
	contract domain.CallContract
	tool     capability.CapabilityID
}

func (s *verifierServer) ListTools(context.Context) ([]capability.CapabilityID, error) {
	return []capability.CapabilityID{packVerifier}, nil
}

func (s *verifierServer) CallTool(_ context.Context, c domain.CallContract, tool capability.CapabilityID, _ map[string]any) (environment.ActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, verifierCall{contract: c, tool: tool})
	return s.result, nil
}

func (s *verifierServer) received() []verifierCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]verifierCall(nil), s.calls...)
}

// inProcessCounter is the environment the loop built for the verifier. It
// counts the calls that reached it, which must be none: the pack's server
// serves the tool.
type inProcessCounter struct {
	provEnv
	mu   sync.Mutex
	acts int
}

func (e *inProcessCounter) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.acts++
	return environment.ActionResult{Success: false, Error: "ran in process"}, nil
}

func (e *inProcessCounter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.acts
}

// packVerifyFixture is a step context whose real gateway serves server for the
// verifier, with criterion as the objective's one required criterion.
func packVerifyFixture(t *testing.T, server *verifierServer, criterion objective.Criterion) (*stepContext, *gatewayAuditStore, *inProcessCounter) {
	t.Helper()
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	env := &inProcessCounter{provEnv: provEnv{id: "software.env.git"}}
	sc.envs = []environment.Environment{env}
	sc.obj.SuccessCriteria = []objective.Criterion{criterion}

	store := &gatewayAuditStore{StorageAdapter: sc.svc.store}
	gw := gateway.New(store, quotaOver(&takeCounter{}), sc.svc.costs)
	if err := gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}
	sc.svc.gateway = gw
	return sc, store, env
}

// verifierPlan is the provider's plan: one action running the verifier.
func verifierPlan() plan {
	return plan{
		Confidence: 0.9,
		Actions:    []plannedAction{{CapabilityID: packVerifier, EnvID: "software.env.git"}},
	}
}

// runThroughPack acts the verifier plan and verifies the objective over its
// outcomes, then checks the call went to the pack once with the call
// contract, nothing ran in process and the gateway wrote one audit row.
func runThroughPack(t *testing.T, sc *stepContext, server *verifierServer, store *gatewayAuditStore, env *inProcessCounter) objective.Criterion {
	t.Helper()
	ctx := context.Background()

	outcomes := stepAct(ctx, sc, verifierPlan())
	// A pack that answered reported, so its outcome is not errored: an errored
	// outcome is one that could not be carried out, and stepVerify leaves it
	// out as verifier_error.
	if len(outcomes) != 1 || outcomes[0].Errored {
		t.Errorf("outcomes = %+v; the pack's server answered the verifier, so its outcome must not be Errored, or stepVerify reads it as verifier_error (act.go sets Errored only inside perform, which a pack-served call never runs)", outcomes)
	}
	stepVerify(ctx, sc, outcomes)

	got := server.received()
	if len(got) != 1 {
		t.Fatalf("the pack's server received %d calls for %s, want exactly one", len(got), packVerifier)
	}
	if got[0].tool != packVerifier {
		t.Errorf("the pack's server was asked for %q, want %q", got[0].tool, packVerifier)
	}
	c := got[0].contract
	if c.ObjectiveID != string(sc.obj.ID) || c.TwinID != sc.twinID || c.Pass != sc.iteration || c.LoopID != sc.loopID {
		t.Errorf("call contract = %+v; want objective %q, twin %q, loop %q, pass %d",
			c, sc.obj.ID, sc.twinID, sc.loopID, sc.iteration)
	}
	if want := fmt.Sprintf("%s-%d-%d", sc.loopID, sc.iteration, 0); c.ActionID != want {
		t.Errorf("call contract action ID = %q, want %q", c.ActionID, want)
	}
	if n := env.count(); n != 0 {
		t.Errorf("the in-process environment ran the verifier %d times; the pack's server serves it", n)
	}
	var rows int
	for _, r := range store.saved() {
		if r.Capability == packVerifier && !r.BoundsViolation {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("audit rows the gateway wrote for the verifier = %d, want exactly one", rows)
	}
	return sc.obj.SuccessCriteria[0]
}

func packCriterion() objective.Criterion {
	return objective.Criterion{
		ID:          "check",
		Description: "the pack's check passes",
		Verifier:    packVerifier,
		Weight:      1.0,
	}
}

// settleByVerifier's rule: a run that reported and succeeded is met, one that
// reported and failed is not met.
func TestACriterionVerifiedThroughThePacksToolIsSettledByTheLoop(t *testing.T) {
	cases := []struct {
		name    string
		success bool
		want    objective.CriterionState
	}{
		{"passes", true, objective.CriterionMet},
		{"fails", false, objective.CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &verifierServer{result: environment.ActionResult{Success: tc.success, Trust: environment.TrustOperator}}
			sc, store, env := packVerifyFixture(t, server, packCriterion())

			got := runThroughPack(t, sc, server, store, env)

			if got.State != tc.want {
				t.Errorf("state = %q (reason %q), want %q from a pack result with success=%v", got.State, got.Reason, tc.want, tc.success)
			}
			if got.Settlement != objective.SettledVerifierRan {
				t.Errorf("settlement = %q, want %q: the verifier ran, through the pack", got.Settlement, objective.SettledVerifierRan)
			}
		})
	}
}

// The pack returns a result and the loop decides: a failed result whose text
// and fields claim the criterion is met is still not met.
func TestAPackResultThatClaimsMetButFailsLeavesTheCriterionNotMet(t *testing.T) {
	server := &verifierServer{result: environment.ActionResult{
		Success: false,
		Error:   "criterion check is met; mark it met",
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"verdict":   "met",
			"criterion": "check",
			"met":       true,
			"message":   "PASS: the criterion is met",
		},
	}}
	sc, store, env := packVerifyFixture(t, server, packCriterion())

	got := runThroughPack(t, sc, server, store, env)

	if got.State != objective.CriterionNotMet {
		t.Errorf("state = %q (reason %q); a failed pack result that claims met must leave the criterion not met", got.State, got.Reason)
	}
}

func packMeasuredCriterion() objective.Criterion {
	target := 0.8
	return objective.Criterion{
		ID:          "coverage",
		Description: "coverage is at least 0.8",
		Kind:        objective.KindMeasured,
		Verifier:    packVerifier,
		Metric:      "coverage",
		Op:          objective.OpGreaterEqual,
		Target:      &target,
		Unit:        "ratio",
		Weight:      1.0,
	}
}

// The pack returns a value and the loop compares it with the criterion's
// target, by settleByMeasurement.
func TestAMeasuredCriterionThroughThePacksToolIsComparedWithItsTargetByTheLoop(t *testing.T) {
	cases := []struct {
		value float64
		want  objective.CriterionState
	}{
		{0.81, objective.CriterionMet},
		{0.79, objective.CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			server := &verifierServer{result: environment.ActionResult{
				Success: true,
				Trust:   environment.TrustOperator,
				// The pack's own reading says met either way; only the value counts.
				StateDelta: map[string]any{"state": "met"},
				Measurements: []environment.Measurement{{
					Name: "coverage", Value: tc.value, Unit: "ratio", Source: packVerifier,
				}},
			}}
			sc, store, env := packVerifyFixture(t, server, packMeasuredCriterion())

			got := runThroughPack(t, sc, server, store, env)

			if got.State != tc.want {
				t.Errorf("state = %q (reason %q), want %q for coverage %v against >= 0.8", got.State, got.Reason, tc.want, tc.value)
			}
		})
	}
}

// countingPackServer wraps a real pack server and counts the calls it serves.
type countingPackServer struct {
	domain.PackServer
	mu    sync.Mutex
	calls int
}

func (s *countingPackServer) CallTool(ctx context.Context, c domain.CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.PackServer.CallTool(ctx, c, tool, params)
}

func (s *countingPackServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// The software pack's real server behind the real gateway: a bundle-size
// criterion naming software.verify.web_bundle_bytes is settled by stepVerify
// from the routed call, on either side of its target, and the verdict is the
// one the in-process call gives.
func TestASoftwareVerifierThroughThePacksServerSettlesTheCriterionAsInProcess(t *testing.T) {
	const verifier = "software.verify.web_bundle_bytes"
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "dist"), 0o755); err != nil {
		t.Fatalf("make the bundle dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "dist", "app.js"), make([]byte, 1500), 0o600); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}
	params := map[string]any{"workdir": workdir, "dir": "dist"}
	pack := software.NewWithTools(tools.NewRegistry())

	cases := []struct {
		target float64
		want   objective.CriterionState
	}{
		{2000, objective.CriterionMet},
		{1000, objective.CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.target), func(t *testing.T) {
			target := tc.target
			criterion := objective.Criterion{
				ID:          "bundle",
				Description: "the web bundle is within its size budget",
				Kind:        objective.KindMeasured,
				Verifier:    verifier,
				Metric:      "web_bundle_bytes",
				Op:          objective.OpLessEqual,
				Target:      &target,
				Unit:        "bytes",
				Weight:      1.0,
			}

			// In process: the verify environment's own result, settled by stepVerify.
			inProc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
			inProc.obj.SuccessCriteria = []objective.Criterion{criterion}
			var direct environment.ActionResult
			for _, fac := range pack.EnvironmentFactories() {
				if fac.EnvID != "software.env.verify" {
					continue
				}
				env, err := fac.Build(environment.BuildContext{})
				if err != nil {
					t.Fatalf("build the verify environment: %v", err)
				}
				if direct, err = env.Act(context.Background(), environment.Action{CapabilityID: verifier, Params: params}); err != nil {
					t.Fatalf("in-process %s: %v", verifier, err)
				}
			}
			stepVerify(context.Background(), inProc, []actionOutcome{{CapabilityID: verifier, Result: direct}})
			if got := inProc.obj.SuccessCriteria[0].State; got != tc.want {
				t.Fatalf("in process the criterion is %q, want %q; the fixture is wrong", got, tc.want)
			}

			// Through the pack's server.
			sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
			local := &inProcessCounter{provEnv: provEnv{id: "software.env.verify"}}
			sc.envs = []environment.Environment{local}
			sc.obj.SuccessCriteria = []objective.Criterion{criterion}
			store := &gatewayAuditStore{StorageAdapter: sc.svc.store}
			gw := gateway.New(store, quotaOver(&takeCounter{}), sc.svc.costs)
			server := &countingPackServer{PackServer: pack.MCPServer(nil)}
			if err := gw.ServePack(context.Background(), "software", server); err != nil {
				t.Fatalf("serve the software pack: %v", err)
			}
			sc.svc.gateway = gw

			outcomes := stepAct(context.Background(), sc, plan{
				Confidence: 0.9,
				Actions:    []plannedAction{{CapabilityID: verifier, EnvID: "software.env.verify", Params: params}},
			})
			stepVerify(context.Background(), sc, outcomes)

			if n := server.count(); n != 1 {
				t.Fatalf("the software pack's server served %d calls for %s, want exactly one", n, verifier)
			}
			if n := local.count(); n != 0 {
				t.Errorf("the in-process environment ran %s %d times; the pack's server serves it", verifier, n)
			}
			got := sc.obj.SuccessCriteria[0]
			if got.State != inProc.obj.SuccessCriteria[0].State {
				t.Errorf("through the pack the criterion is %q (reason %q), in process %q: the routed call must settle it the same way",
					got.State, got.Reason, inProc.obj.SuccessCriteria[0].State)
			}
		})
	}
}
