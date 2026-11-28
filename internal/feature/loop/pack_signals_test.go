package loop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// Phase 39 step 4, the orchestrator's half (slice B). The loop knows no
// workspace: actions of one pass routed to a pack share what the pack keeps
// because every call carries the same identifiers, not because the loop
// provisions anything or passes a path between them. The pack releases what it
// keeps on lifecycle signals the orchestrator sends: "pass ended" to every pack
// called during a pass, "objective ended" to every pack called for an
// objective that ended or was cancelled. Delivery is best effort.
//
// The packs here are stubs in neutral terms ("test.*"); none of them is the
// software pack, so nothing below depends on what a worktree is.

const (
	signalEnv  = "test.env.pack"
	toolCheck  = "test.verify.check"
	toolUnused = "test.observe.unused"
	toolWrite  = "test.act.write"
	toolRun    = "test.verify.run"
)

// workspaceKeys are the params and result fields the loop used to add for a
// capability that writes files. A call routed to a pack carries none of them,
// in either direction.
var workspaceKeys = []string{"worktree_path", "workdir", "planned_branch", "workspace", "workspace_ref"}

// lifecyclePack is a pack's server that keeps "files" per pass, keyed on the
// call contract's identifiers alone, and records the calls and lifecycle
// signals it receives. A write stores params.content; a run reports the
// contents its pass holds. When block is set, a tool call holds until its
// context is cancelled, like a long test run.
type lifecyclePack struct {
	tools       []capability.CapabilityID
	block       bool
	failSignals bool
	started     chan struct{}

	mu      sync.Mutex
	files   map[string][]string
	calls   []lifecycleCall
	signals []lifecycleCall
}

type lifecycleCall struct {
	contract domain.CallContract
	tool     capability.CapabilityID
	params   map[string]any
}

func newLifecyclePack(tools ...capability.CapabilityID) *lifecyclePack {
	return &lifecyclePack{tools: tools, started: make(chan struct{}, 16), files: map[string][]string{}}
}

// passKey is everything in the contract that names a pass; the action and the
// bounds differ between calls of one pass.
func passKey(c domain.CallContract) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d", c.TenantID, c.TwinID, c.ObjectiveID, c.LoopID, c.Pass)
}

func (p *lifecyclePack) ListTools(context.Context) ([]capability.CapabilityID, error) {
	return p.tools, nil
}

func (p *lifecyclePack) CallTool(ctx context.Context, c domain.CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error) {
	p.mu.Lock()
	if tool == domain.ToolPassEnded || tool == domain.ToolObjectiveEnded {
		p.signals = append(p.signals, lifecycleCall{contract: c, tool: tool, params: params})
		p.mu.Unlock()
		if p.failSignals {
			return environment.ActionResult{Success: false, Error: "the pack is down"}, errors.New("the pack is down")
		}
		return environment.ActionResult{Success: true, Trust: environment.TrustOperator}, nil
	}
	p.calls = append(p.calls, lifecycleCall{contract: c, tool: tool, params: params})
	key := passKey(c)
	var result environment.ActionResult
	switch tool {
	case toolWrite:
		content, _ := params["content"].(string)
		p.files[key] = append(p.files[key], content)
		result = environment.ActionResult{Success: true, Trust: environment.TrustOperator}
	case toolRun:
		seen := append([]string(nil), p.files[key]...)
		result = environment.ActionResult{Success: len(seen) > 0, Trust: environment.TrustOperator, StateDelta: map[string]any{"seen": seen}}
	default:
		result = environment.ActionResult{Success: true, Trust: environment.TrustOperator}
	}
	p.mu.Unlock()

	if p.block {
		p.started <- struct{}{}
		<-ctx.Done()
		return environment.ActionResult{}, ctx.Err()
	}
	return result, nil
}

func (p *lifecyclePack) received(kind capability.CapabilityID) []lifecycleCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []lifecycleCall
	for _, s := range p.signals {
		if s.tool == kind {
			out = append(out, s)
		}
	}
	return out
}

func (p *lifecyclePack) toolCalls() []lifecycleCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]lifecycleCall(nil), p.calls...)
}

// waitSignals polls until the pack received n signals of kind or stopBound
// passes: a stop returns before its loop has unwound.
func (p *lifecyclePack) waitSignals(kind capability.CapabilityID, n int) []lifecycleCall {
	deadline := time.Now().Add(stopBound)
	for {
		got := p.received(kind)
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// signalPlan is a provider reply planning one action per tool, each run by
// the pack's environment.
func signalPlan(actions ...string) string {
	return `{"actions":[` + strings.Join(actions, ",") + `],"confidence":0.95,"reasoning":"r"}`
}

func plannedTool(tool string) string {
	return fmt.Sprintf(`{"capability":%q,"env_id":%q,"reason":"r"}`, tool, signalEnv)
}

// packLoopFixture is a service that runs the whole loop over a scripted plan
// whose actions the packs' servers serve, through a real gateway. The
// environment the loop builds for them counts what reaches it in process,
// which must be nothing.
func packLoopFixture(t *testing.T, reply string, packs ...*lifecyclePack) (*serviceImpl, coreloop.Request, *inProcessCounter) {
	t.Helper()
	svc, _, obj := traceLoopFixture(t)
	svc.tracer = nil

	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: reply})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	var served []capability.CapabilityID
	for _, p := range packs {
		served = append(served, p.tools...)
	}
	env := &inProcessCounter{provEnv: provEnv{id: signalEnv}}
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  signalEnv,
		Domain: "test",
		Serves: served,
		Build:  func(environment.BuildContext) (environment.Environment, error) { return env, nil },
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	gw := gateway.New(svc.store, karakuriquota.Deps{}, svc.costs)
	for i, p := range packs {
		if err := gw.ServePack(context.Background(), fmt.Sprintf("pack-%d", i), p); err != nil {
			t.Fatalf("serve pack %d: %v", i, err)
		}
	}
	svc.gateway = gw

	// One pass: the scripted provider plans the same action every pass, and
	// the loop pauses a repeated plan at a checkpoint, which is not what these
	// tests are about.
	return svc, coreloop.Request{Objective: obj, Agent: traceAgent(unlimited), MaxIter: 1}, env
}

const signalLoopID = "loop-signals-0001"

// runBounded drives the loop on its own goroutine and fails the test when it
// has not returned within stopBound, rather than letting a pause hang it.
func runBounded(t *testing.T, svc *serviceImpl, req coreloop.Request) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSync(svc, signalLoopID, req, nil)
	}()
	select {
	case <-done:
	case <-time.After(stopBound):
		st, _ := svc.Status(context.Background(), signalLoopID)
		t.Fatalf("the loop did not end within %s: status %+v", stopBound, st)
	}
}

// assertSignalNames checks a signal names the run by the orchestrator's own
// identifiers and carries no params: nothing a pack defined comes back.
func assertSignalNames(t *testing.T, sig lifecycleCall, obj objective.Objective, loopID string, pass int, withPass bool) {
	t.Helper()
	c := sig.contract
	if c.ObjectiveID != string(obj.ID) || c.TwinID != obj.TwinID {
		t.Errorf("%s contract = %+v; want objective %q and twin %q", sig.tool, c, obj.ID, obj.TwinID)
	}
	if withPass && (c.LoopID != loopID || c.Pass != pass) {
		t.Errorf("%s contract = %+v; want loop %q and pass %d", sig.tool, c, loopID, pass)
	}
	if len(sig.params) != 0 {
		t.Errorf("%s params = %v; a signal carries the orchestrator's identifiers and nothing a pack defined", sig.tool, sig.params)
	}
}

func TestTheLoopSignalsPassEndedToEveryPackItCalledAfterEachPass(t *testing.T) {
	called := newLifecyclePack(toolCheck)
	idle := newLifecyclePack(toolUnused)
	svc, req, env := packLoopFixture(t, signalPlan(plannedTool(toolCheck)), called, idle)

	runBounded(t, svc, req)

	calls := called.toolCalls()
	if len(calls) != 1 {
		t.Fatalf("the pack received %d tool calls over one pass, want one; the fixture does not route the plan to it", len(calls))
	}
	if n := env.count(); n != 0 {
		t.Errorf("the in-process environment ran %d actions; the pack serves them", n)
	}
	got := called.received(domain.ToolPassEnded)
	if len(got) != 1 {
		t.Fatalf("the pack called during the pass received %d pass-ended signals, want exactly one", len(got))
	}
	assertSignalNames(t, got[0], req.Objective, signalLoopID, calls[0].contract.Pass, true)
	if got := idle.received(domain.ToolPassEnded); len(got) != 0 {
		t.Errorf("a pack no pass called received pass-ended %+v; only packs called during the pass are signalled", got)
	}
}

func TestAOneShotObjectiveThatEndsSignalsObjectiveEndedToEveryPackItCalled(t *testing.T) {
	called := newLifecyclePack(toolCheck)
	idle := newLifecyclePack(toolUnused)
	svc, req, _ := packLoopFixture(t, signalPlan(plannedTool(toolCheck)), called, idle)

	runBounded(t, svc, req)

	if st := storedLoop(t, svc.store, signalLoopID); !st.Completed {
		t.Fatalf("the run did not end: %+v", st)
	}
	got := called.received(domain.ToolObjectiveEnded)
	if len(got) != 1 {
		t.Fatalf("the pack called for the objective received %d objective-ended signals when its one-shot run ended, want exactly one", len(got))
	}
	assertSignalNames(t, got[0], req.Objective, "", 0, false)
	if got := idle.received(domain.ToolObjectiveEnded); len(got) != 0 {
		t.Errorf("a pack never called for the objective received objective-ended %+v", got)
	}
}

// A standing objective does not end when one of its passes does: reconcile
// runs the next pass, so its packs hear "pass ended" and nothing more.
func TestAStandingObjectivesPassEndingIsNotItsEnd(t *testing.T) {
	called := newLifecyclePack(toolCheck)
	svc, req, _ := packLoopFixture(t, signalPlan(plannedTool(toolCheck)), called)
	req.Objective = standing(t, svc, req.Objective)
	req.MaxIter = 1

	runBounded(t, svc, req)

	if got := called.received(domain.ToolPassEnded); len(got) != 1 {
		t.Errorf("the pack received %d pass-ended signals for the standing objective's one pass, want one", len(got))
	}
	if got := called.received(domain.ToolObjectiveEnded); len(got) != 0 {
		t.Errorf("the pack received objective-ended %+v when a standing objective's pass ended; the objective goes on", got)
	}
}

func TestCancellingAnObjectiveSignalsEveryPackItCalled(t *testing.T) {
	called := newLifecyclePack(toolCheck)
	called.block = true
	idle := newLifecyclePack(toolUnused)
	svc, req, _ := packLoopFixture(t, signalPlan(plannedTool(toolCheck)), called, idle)
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-called.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the pack's tool call never started")
	}
	if _, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := called.waitSignals(domain.ToolObjectiveEnded, 1)
	if len(got) != 1 {
		t.Fatalf("the pack called for the cancelled objective received %d objective-ended signals within %s of the stop, want exactly one", len(got), stopBound)
	}
	assertSignalNames(t, got[0], req.Objective, "", 0, false)
	time.Sleep(stopSettle)
	if got := idle.received(domain.ToolObjectiveEnded); len(got) != 0 {
		t.Errorf("a pack never called for the cancelled objective received %+v", got)
	}
}

// A pack that fails every signal neither fails the run nor is asked again: one
// attempt per pass ended, one for the objective, and the run ends as it would
// have without the packs.
func TestAFailedSignalDoesNotFailThePassAndIsNotRetried(t *testing.T) {
	called := newLifecyclePack(toolCheck)
	called.failSignals = true
	svc, req, _ := packLoopFixture(t, signalPlan(plannedTool(toolCheck)), called)

	runBounded(t, svc, req)

	st := storedLoop(t, svc.store, signalLoopID)
	if st.Outcome == coreloop.RunErrored || st.Error != "" {
		t.Errorf("stored outcome %q, error %q; a signal that fails to deliver must not fail the run", st.Outcome, st.Error)
	}
	if st.Outcome != coreloop.RunBudgetSpent {
		t.Errorf("stored outcome = %q, want %q: the run ends as it would have with no pack", st.Outcome, coreloop.RunBudgetSpent)
	}
	time.Sleep(200 * time.Millisecond) // long enough for a background retry to show
	if got := called.received(domain.ToolPassEnded); len(got) != req.MaxIter {
		t.Errorf("the failing pack was sent pass-ended %d times over %d passes, want one attempt per pass", len(got), req.MaxIter)
	}
	if got := called.received(domain.ToolObjectiveEnded); len(got) != 1 {
		t.Errorf("the failing pack was sent objective-ended %d times, want one attempt", len(got))
	}
}

// shareFixture is a step context whose capability registry says the write
// changes something, and whose gateway routes both the write and the run to
// pack.
func shareFixture(t *testing.T, pack *lifecyclePack, loopID string) (*stepContext, *inProcessCounter) {
	t.Helper()
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	sc.loopID = loopID
	sc.state.id = loopID
	env := &inProcessCounter{provEnv: provEnv{id: signalEnv}}
	sc.envs = []environment.Environment{env}

	caps := capability.NewRegistry()
	for _, c := range []capability.Capability{
		{ID: toolWrite, Name: "Write", Domain: "test", Writes: true},
		{ID: toolRun, Name: "Run", Domain: "test", Verifiable: true},
	} {
		if err := caps.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	sc.svc.capReg = caps

	gw := gateway.New(sc.svc.store, karakuriquota.Deps{}, sc.svc.costs)
	if err := gw.ServePack(context.Background(), "test", pack); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}
	sc.svc.gateway = gw
	return sc, env
}

func writeThenRun(content string) plan {
	return plan{
		Confidence: 0.9,
		Actions: []plannedAction{
			{CapabilityID: toolWrite, EnvID: signalEnv, Params: map[string]any{"content": content}},
			{CapabilityID: toolRun, EnvID: signalEnv},
		},
	}
}

// assertNoWorkspaceField fails when a map the loop sent or kept names a
// workspace.
func assertNoWorkspaceField(t *testing.T, where string, m map[string]any) {
	t.Helper()
	for _, k := range workspaceKeys {
		if _, ok := m[k]; ok {
			t.Errorf("%s carries %q = %v; the orchestrator passes no workspace between actions", where, k, m[k])
		}
	}
}

func TestAWriteAndATestRunInOnePassShareFilesWithoutTheLoopPassingAnything(t *testing.T) {
	pack := newLifecyclePack(toolWrite, toolRun)
	sc, env := shareFixture(t, pack, "loop-share-0001")

	outcomes := stepAct(context.Background(), sc, writeThenRun("written in this pass"))

	calls := pack.toolCalls()
	if len(calls) != 2 || calls[0].tool != toolWrite || calls[1].tool != toolRun {
		t.Fatalf("the pack received %+v; want the write and then the run, both routed to it", calls)
	}
	if passKey(calls[0].contract) != passKey(calls[1].contract) {
		t.Errorf("the write's contract %+v and the run's %+v name different passes; actions of one pass carry the same identifiers", calls[0].contract, calls[1].contract)
	}
	if calls[0].contract.ActionID == calls[1].contract.ActionID {
		t.Errorf("the write and the run carry the same action ID %q", calls[0].contract.ActionID)
	}
	if want := map[string]any{"content": "written in this pass"}; !reflect.DeepEqual(calls[0].params, want) {
		t.Errorf("the write reached the pack with params %v, want the plan's own %v and nothing the loop added", calls[0].params, want)
	}
	if len(calls[1].params) != 0 {
		t.Errorf("the run reached the pack with params %v; the plan gave it none, and the loop adds no path or reference", calls[1].params)
	}
	if n := env.count(); n != 0 {
		t.Errorf("the in-process environment ran %d actions; the pack serves both", n)
	}

	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %+v, want two", outcomes)
	}
	run := outcomes[1].Result
	if !run.Success || !reflect.DeepEqual(run.StateDelta["seen"], []string{"written in this pass"}) {
		t.Errorf("the run's result = %+v; it must see the write of its own pass", run)
	}
	for i, o := range outcomes {
		assertNoWorkspaceField(t, fmt.Sprintf("outcome %d params", i), o.Params)
		assertNoWorkspaceField(t, fmt.Sprintf("outcome %d result", i), o.Result.StateDelta)
	}
}

func TestTwoConcurrentPassesDoNotShareFiles(t *testing.T) {
	pack := newLifecyclePack(toolWrite, toolRun)
	one, _ := shareFixture(t, pack, "loop-share-0001")
	two := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	two.svc = one.svc
	two.envs = one.envs
	two.obj = one.obj
	two.loopID = "loop-share-0002"
	two.state.id = two.loopID

	var wg sync.WaitGroup
	results := make([][]actionOutcome, 2)
	for i, sc := range []*stepContext{one, two} {
		wg.Add(1)
		go func(i int, sc *stepContext) {
			defer wg.Done()
			results[i] = stepAct(context.Background(), sc, writeThenRun("written by "+sc.loopID))
		}(i, sc)
	}
	wg.Wait()

	if n := len(pack.toolCalls()); n != 4 {
		t.Fatalf("the pack received %d tool calls from two passes of a write and a run, want four", n)
	}
	for i, sc := range []*stepContext{one, two} {
		if len(results[i]) != 2 {
			t.Fatalf("pass %s outcomes = %+v, want two", sc.loopID, results[i])
		}
		want := []string{"written by " + sc.loopID}
		if got := results[i][1].Result.StateDelta["seen"]; !reflect.DeepEqual(got, want) {
			t.Errorf("the run of %s saw %v, want only its own pass's write %v", sc.loopID, got, want)
		}
	}
}
