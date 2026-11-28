package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// Phase 39 step 4, slice S2: the loop stops provisioning. A write_code action
// that reaches an environment through the gateway is handed to that
// environment with the plan's params and nothing else; the orchestrator holds
// no WorktreeManager, creates no worktree, saves no worktree record and
// publishes no worktree_created event. The pack that serves a file-writing
// tool keeps its own worktrees (slice S1).

const noProvisionWrite = "software.act.write_code"

// paramsRecorder is an environment that keeps the params of every action it
// is asked to run.
type paramsRecorder struct {
	provEnv
	mu    sync.Mutex
	calls []map[string]any
}

func (e *paramsRecorder) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make(map[string]any, len(a.Params))
	for k, v := range a.Params {
		cp[k] = v
	}
	e.calls = append(e.calls, cp)
	return environment.ActionResult{Success: true}, nil
}

func (e *paramsRecorder) received() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]map[string]any(nil), e.calls...)
}

func TestAWriteActionRunsWithNoWorktreeProvisionedByTheLoop(t *testing.T) {
	ctx := context.Background()
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	// Long enough for any ID the loop derives from it.
	sc.loopID = "loop-noprovision-0001"
	sc.state.id = sc.loopID
	env := &paramsRecorder{provEnv: provEnv{id: "software.env.git"}}
	sc.envs = []environment.Environment{env}

	// The capability as the software pack declared it before step 4, with the
	// needs_workspace key the loop used to read. Decoded rather than built so
	// the test compiles once the field leaves the core type: from then on the
	// key is ignored, which is the point.
	var c capability.Capability
	if err := json.Unmarshal([]byte(`{"id":"`+noProvisionWrite+`","name":"Write code","domain":"software","needs_workspace":true}`), &c); err != nil {
		t.Fatalf("decode capability: %v", err)
	}
	caps := capability.NewRegistry()
	if err := caps.Register(c); err != nil {
		t.Fatalf("register %s: %v", c.ID, err)
	}
	sc.svc.capReg = caps
	// No WorktreeManager is wired: the loop has none to call.
	sc.svc.gateway = gateway.New(sc.svc.store, karakuriquota.Deps{}, sc.svc.costs)

	events, unsub := sc.svc.hub.Subscribe(ctx, "obj:"+string(sc.obj.ID))
	defer unsub()

	p := plan{
		Confidence: 0.9,
		Actions: []plannedAction{{
			CapabilityID: noProvisionWrite,
			EnvID:        "software.env.git",
			Params:       map[string]any{"prompt": "write the handler", "branch": "karakuri/feature-x"},
		}},
	}
	outcomes, panicked := actRecovering(ctx, sc, p)
	if panicked != nil {
		t.Fatalf("the act step reached for a WorktreeManager the orchestrator does not hold; the loop provisions no worktree, the pack that serves the tool does. Panic: %v", panicked)
	}

	calls := env.received()
	if len(calls) != 1 {
		t.Fatalf("the environment received %d write_code calls through the gateway, want exactly one", len(calls))
	}
	for _, k := range []string{"worktree_path", "planned_branch"} {
		if v, ok := calls[0][k]; ok {
			t.Errorf("the environment received params[%q] = %v; the loop adds no workspace to an action", k, v)
		}
	}
	if got := calls[0]["branch"]; got != "karakuri/feature-x" {
		t.Errorf("the environment received params.branch = %v, want the plan's own %q, not a branch the loop made", got, "karakuri/feature-x")
	}
	if len(outcomes) != 1 || outcomes[0].Errored {
		t.Errorf("outcomes = %+v; want one action carried out", outcomes)
	}

	if wts, err := sc.svc.store.ListWorktrees(ctx, sc.obj.ID); err != nil {
		t.Errorf("list worktrees: %v", err)
	} else if len(wts) != 0 {
		t.Errorf("the loop saved %d worktree records (%+v); the orchestrator keeps none", len(wts), wts)
	}

	for {
		select {
		case e := <-events:
			if e.Type == event.TypeWorktreeCreated {
				t.Errorf("the loop published %s %v; the orchestrator creates no worktree", e.Type, e.Payload)
			}
			continue
		default:
		}
		break
	}
}

// actRecovering runs stepAct and returns a panic from it instead of letting
// it end the test binary.
func actRecovering(ctx context.Context, sc *stepContext, p plan) (outcomes []actionOutcome, panicked any) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprintf("%v\n%s", r, debug.Stack())
		}
	}()
	return stepAct(ctx, sc, p), nil
}
