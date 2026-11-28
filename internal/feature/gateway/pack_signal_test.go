package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 39 step 4, the orchestrator's half of the lifecycle signals: the
// gateway, which sees every call a pack receives, sends "pass ended" to every
// pack called during a pass and "objective ended" to every pack called for an
// objective, and to no other pack. The signals are the reserved tools of
// domain.ToolPassEnded and domain.ToolObjectiveEnded (slice A), carried with
// the orchestrator's own identifiers and nothing a pack defined. Delivery is
// best effort: a pack that fails to take one is not retried.

// signalServer is a pack's server serving one tool. It records the lifecycle
// signals it receives apart from its tool calls, and fails every signal when
// failSignals is set.
type signalServer struct {
	tool        capability.CapabilityID
	failSignals bool

	mu      sync.Mutex
	calls   []domain.CallContract
	signals []packCall
}

func (s *signalServer) ListTools(context.Context) ([]capability.CapabilityID, error) {
	return []capability.CapabilityID{s.tool}, nil
}

func (s *signalServer) CallTool(_ context.Context, c domain.CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tool == domain.ToolPassEnded || tool == domain.ToolObjectiveEnded {
		s.signals = append(s.signals, packCall{contract: c, tool: tool, params: params})
		if s.failSignals {
			return environment.ActionResult{Success: false, Error: "the pack is down"}, errors.New("the pack is down")
		}
		return environment.ActionResult{Success: true, Trust: environment.TrustOperator}, nil
	}
	s.calls = append(s.calls, c)
	return environment.ActionResult{Success: true, Trust: environment.TrustOperator}, nil
}

// received returns the signals of kind the server received.
func (s *signalServer) received(kind capability.CapabilityID) []packCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []packCall
	for _, sig := range s.signals {
		if sig.tool == kind {
			out = append(out, sig)
		}
	}
	return out
}

// servePacks registers one signalServer per tool on the harness's gateway.
func servePacks(t *testing.T, h harness, tools ...capability.CapabilityID) map[capability.CapabilityID]*signalServer {
	t.Helper()
	servers := make(map[capability.CapabilityID]*signalServer, len(tools))
	for i, tool := range tools {
		s := &signalServer{tool: tool}
		if err := h.gw.ServePack(context.Background(), fmt.Sprintf("pack-%d", i), s); err != nil {
			t.Fatalf("serve pack for %s: %v", tool, err)
		}
		servers[tool] = s
	}
	return servers
}

// passCall is a loop call to tool in the named objective, loop and pass.
func passCall(tool capability.CapabilityID, objectiveID, loopID string, pass int) Call {
	call := contractCall()
	call.Capability = string(tool)
	call.ObjectiveID = objectiveID
	call.LoopID = loopID
	call.Pass = pass
	call.ActionID = fmt.Sprintf("%s-%d-%s", loopID, pass, tool)
	return call
}

func callOK(t *testing.T, h harness, call Call) {
	t.Helper()
	if _, err := h.gw.Call(context.Background(), call, notInProcess(t)); err != nil {
		t.Fatalf("call %s: %v", call.Capability, err)
	}
}

// contractOf is the call contract the gateway sends for call, which names the
// pass the signal must name.
func contractOf(call Call) domain.CallContract {
	return domain.CallContract{
		TenantID:    call.TenantID,
		TwinID:      call.TwinID,
		ObjectiveID: call.ObjectiveID,
		LoopID:      call.LoopID,
		Pass:        call.Pass,
	}
}

// assertNamesPass checks a signal names the pass by the orchestrator's own
// identifiers and carries nothing a pack defined: no params at all.
func assertNamesPass(t *testing.T, who string, sig packCall, want domain.CallContract) {
	t.Helper()
	c := sig.contract
	if c.TenantID != want.TenantID || c.TwinID != want.TwinID || c.ObjectiveID != want.ObjectiveID || c.LoopID != want.LoopID || c.Pass != want.Pass {
		t.Errorf("%s: signal contract = %+v; want tenant %q, twin %q, objective %q, loop %q, pass %d",
			who, c, want.TenantID, want.TwinID, want.ObjectiveID, want.LoopID, want.Pass)
	}
	if len(sig.params) != 0 {
		t.Errorf("%s: signal params = %v; a signal carries the orchestrator's identifiers and nothing a pack defined", who, sig.params)
	}
}

const (
	toolA capability.CapabilityID = "alpha.observe.read"
	toolB capability.CapabilityID = "beta.verify.check"
	toolC capability.CapabilityID = "gamma.observe.list"
)

func TestPassEndedReachesEveryPackCalledDuringThePassAndNoOther(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA, toolB, toolC)

	inPass := passCall(toolA, "obj-1", "loop-1", 0)
	callOK(t, h, inPass)
	callOK(t, h, passCall(toolB, "obj-1", "loop-1", 0))
	callOK(t, h, passCall(toolA, "obj-1", "loop-1", 0)) // a pack called twice is signalled once
	callOK(t, h, passCall(toolC, "obj-1", "loop-1", 1)) // the next pass

	h.gw.PassEnded(context.Background(), contractOf(inPass))

	for _, tool := range []capability.CapabilityID{toolA, toolB} {
		got := packs[tool].received(domain.ToolPassEnded)
		if len(got) != 1 {
			t.Errorf("the pack serving %s, called during the pass, received %d pass-ended signals, want exactly one", tool, len(got))
			continue
		}
		assertNamesPass(t, string(tool), got[0], contractOf(inPass))
	}
	if got := packs[toolC].received(domain.ToolPassEnded); len(got) != 0 {
		t.Errorf("the pack serving %s, called only in another pass, received %+v; only packs called during the pass are signalled", toolC, got)
	}
	for tool, s := range packs {
		if got := s.received(domain.ToolObjectiveEnded); len(got) != 0 {
			t.Errorf("the pack serving %s received objective-ended %+v when only a pass ended", tool, got)
		}
	}
}

func TestPassEndedForAPassThatCalledNoPackSendsNothing(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA)
	callOK(t, h, passCall(toolA, "obj-1", "loop-1", 0))

	h.gw.PassEnded(context.Background(), contractOf(passCall(toolA, "obj-1", "loop-2", 0)))
	h.gw.PassEnded(context.Background(), contractOf(passCall(toolA, "obj-2", "loop-1", 0)))

	if got := packs[toolA].received(domain.ToolPassEnded); len(got) != 0 {
		t.Errorf("the pack received %+v for passes that never called it", got)
	}
}

func TestObjectiveEndedReachesEveryPackCalledForTheObjectiveAndNoOther(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA, toolB, toolC)

	first := passCall(toolA, "obj-1", "loop-1", 0)
	callOK(t, h, first)
	callOK(t, h, passCall(toolB, "obj-1", "loop-2", 3)) // another loop of the same objective
	callOK(t, h, passCall(toolC, "obj-2", "loop-3", 0)) // another objective

	// A pass that ended first does not make the gateway forget the objective's
	// packs: the objective's ending still reaches them.
	h.gw.PassEnded(context.Background(), contractOf(first))

	ended := domain.CallContract{TenantID: first.TenantID, TwinID: first.TwinID, ObjectiveID: "obj-1"}
	h.gw.ObjectiveEnded(context.Background(), ended)

	for _, tool := range []capability.CapabilityID{toolA, toolB} {
		got := packs[tool].received(domain.ToolObjectiveEnded)
		if len(got) != 1 {
			t.Errorf("the pack serving %s, called for the objective, received %d objective-ended signals, want exactly one", tool, len(got))
			continue
		}
		if got[0].contract.ObjectiveID != "obj-1" || got[0].contract.TwinID != first.TwinID || got[0].contract.TenantID != first.TenantID {
			t.Errorf("%s: objective-ended contract = %+v; want objective obj-1, twin %q, tenant %q", tool, got[0].contract, first.TwinID, first.TenantID)
		}
		if len(got[0].params) != 0 {
			t.Errorf("%s: objective-ended params = %v; a signal carries nothing a pack defined", tool, got[0].params)
		}
	}
	if got := packs[toolC].received(domain.ToolObjectiveEnded); len(got) != 0 {
		t.Errorf("the pack serving %s, called only for another objective, received %+v", toolC, got)
	}
}

// Two passes running at the same time each call the same pack; the end of one
// is signalled for that pass alone, so the pack cannot release the other's.
func TestConcurrentPassesAreSignalledApart(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA)

	one := passCall(toolA, "obj-1", "loop-1", 0)
	two := passCall(toolA, "obj-1", "loop-2", 0)
	var wg sync.WaitGroup
	for _, call := range []Call{one, two} {
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(c Call) {
				defer wg.Done()
				_, _ = h.gw.Call(context.Background(), c, notInProcess(t))
			}(call)
		}
	}
	wg.Wait()

	h.gw.PassEnded(context.Background(), contractOf(one))

	got := packs[toolA].received(domain.ToolPassEnded)
	if len(got) != 1 {
		t.Fatalf("the pack received %d pass-ended signals after one of two concurrent passes ended, want exactly one", len(got))
	}
	assertNamesPass(t, "concurrent", got[0], contractOf(one))
}

// A pack that fails to take a signal neither stops the others from getting
// theirs nor is tried again: one attempt per signal, and nothing left running
// to retry it.
func TestAFailedSignalIsBestEffortAndNotRetried(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA, toolB)
	packs[toolA].failSignals = true

	call := passCall(toolA, "obj-1", "loop-1", 0)
	callOK(t, h, call)
	callOK(t, h, passCall(toolB, "obj-1", "loop-1", 0))

	done := make(chan struct{})
	go func() {
		h.gw.PassEnded(context.Background(), contractOf(call))
		h.gw.ObjectiveEnded(context.Background(), domain.CallContract{TenantID: call.TenantID, TwinID: call.TwinID, ObjectiveID: "obj-1"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the signals did not return within 5s of a pack failing one; delivery is best effort and must not hold the caller")
	}

	// Long enough for a background retry to show itself.
	time.Sleep(200 * time.Millisecond)

	for _, kind := range []capability.CapabilityID{domain.ToolPassEnded, domain.ToolObjectiveEnded} {
		if got := packs[toolA].received(kind); len(got) != 1 {
			t.Errorf("the failing pack received %s %d times, want exactly one attempt: a failed signal is not retried", kind, len(got))
		}
		if got := packs[toolB].received(kind); len(got) != 1 {
			t.Errorf("the healthy pack received %s %d times after the other pack failed it, want exactly one", kind, len(got))
		}
	}
}

// A signal is the gateway's own message to a pack, not a tool call a caller
// made: it is not charged against the twin's capability allowance, so a twin
// whose allowance is spent still lets its packs release what they hold.
func TestASignalIsSentWhenTheCapabilityAllowanceIsSpent(t *testing.T) {
	h := newHarness(true)
	packs := servePacks(t, h, toolA)
	call := passCall(toolA, "obj-1", "loop-1", 0)
	callOK(t, h, call)
	taken := h.backend.taken()

	h.backend.allow = false
	h.gw.PassEnded(context.Background(), contractOf(call))

	if got := packs[toolA].received(domain.ToolPassEnded); len(got) != 1 {
		t.Errorf("the pack received %d pass-ended signals with the allowance spent, want exactly one", len(got))
	}
	if n := h.backend.taken(); n != taken {
		t.Errorf("quota charges went from %d to %d on a signal; a signal is not a tool call the twin pays for", taken, n)
	}
}
