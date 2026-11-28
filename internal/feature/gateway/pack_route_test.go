package gateway

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 39 step 3: the gateway routes a call for a tool a pack's server lists
// to that server, sends it the call contract's fields (ADR 028 section b), and
// marks a result that states no trust as third party.

// packCall is one call a recordingServer received.
type packCall struct {
	contract domain.CallContract
	tool     capability.CapabilityID
	params   map[string]any
}

// recordingServer is a pack's server that records what it was sent and
// answers with a fixed result.
type recordingServer struct {
	tools  []capability.CapabilityID
	result environment.ActionResult

	mu    sync.Mutex
	calls []packCall
}

func (s *recordingServer) ListTools(context.Context) ([]capability.CapabilityID, error) {
	return s.tools, nil
}

func (s *recordingServer) CallTool(_ context.Context, c domain.CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, packCall{contract: c, tool: tool, params: params})
	return s.result, nil
}

func (s *recordingServer) received() []packCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]packCall(nil), s.calls...)
}

// contractCall is a loop call carrying every field of the call contract, each
// set to a value no other field holds.
func contractCall() Call {
	call := callFrom(CallerLoop)
	call.TenantID = "tenant-7"
	call.LoopID = "loop-3"
	call.Pass = 4
	call.ActionID = "action-9"
	call.Bounds = agent.AuthorityBounds{
		MaxAutonomousActions: 5,
		RequiresApprovalFor:  []capability.CapabilityID{"software.act.create_pr"},
		ConfidenceThreshold:  0.7,
		ActionLimits:         []agent.ActionLimit{{Capability: "software.act.write_code", Parameter: "paths", Patterns: []string{"internal/*"}}},
		RefuseOutsideLimits:  true,
	}
	return call
}

func notInProcess(t *testing.T) Perform {
	t.Helper()
	return func(context.Context) (environment.ActionResult, error) {
		t.Error("a call for a tool the pack's server serves ran on the in-process path")
		return environment.ActionResult{Success: false, Error: "ran in process"}, nil
	}
}

func TestTheGatewaySendsAPackToolCallEveryFieldOfTheCallContract(t *testing.T) {
	h := newHarness(true)
	server := &recordingServer{
		tools:  []capability.CapabilityID{"software.observe.fetch_prs"},
		result: environment.ActionResult{Success: true, Trust: environment.TrustOperator, StateDelta: map[string]any{"count": 0}},
	}
	if err := h.gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}
	call := contractCall()

	result, err := h.gw.Call(context.Background(), call, notInProcess(t))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !result.Success {
		t.Errorf("result = %+v, want the server's success", result)
	}

	got := server.received()
	if len(got) != 1 {
		t.Fatalf("the server received %d calls, want exactly one", len(got))
	}
	want := domain.CallContract{
		TenantID:    call.TenantID,
		TwinID:      call.TwinID,
		ObjectiveID: call.ObjectiveID,
		LoopID:      call.LoopID,
		Pass:        call.Pass,
		ActionID:    call.ActionID,
		Bounds:      call.Bounds,
	}
	if !reflect.DeepEqual(got[0].contract, want) {
		t.Errorf("the server saw the contract %+v, want %+v", got[0].contract, want)
	}
	if got[0].tool != "software.observe.fetch_prs" {
		t.Errorf("the server was asked for %q, want software.observe.fetch_prs", got[0].tool)
	}
	if got[0].params["state"] != "open" {
		t.Errorf("the server saw params %v, want the call's own", got[0].params)
	}
	// Routing to a pack does not take the call off the gateway's books.
	if rows := h.store.saved(); len(rows) != 1 {
		t.Errorf("audit rows = %d, want exactly one for the routed call", len(rows))
	}
	if n := h.backend.taken(); n != 1 {
		t.Errorf("quota charges = %d, want exactly one for the routed call", n)
	}
}

func TestAPackResultThatStatesNoTrustIsRecordedAsThirdParty(t *testing.T) {
	h := newHarness(true)
	server := &recordingServer{
		tools:  []capability.CapabilityID{"software.observe.fetch_prs"},
		result: environment.ActionResult{Success: true, StateDelta: map[string]any{"title": "a pull request title somebody wrote"}},
	}
	if err := h.gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}

	result, err := h.gw.Call(context.Background(), contractCall(), notInProcess(t))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(server.received()) != 1 {
		t.Fatalf("the server received %d calls, want the routed one", len(server.received()))
	}
	if result.Trust != environment.TrustThirdParty {
		t.Errorf("trust = %q; a pack result with no trust stated must be treated as third party", result.Trust)
	}

	rows := h.store.saved()
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want exactly one", len(rows))
	}
	var payload struct {
		Result environment.ActionResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("audit payload %s: %v", rows[0].PayloadJSON, err)
	}
	if payload.Result.Trust != environment.TrustThirdParty {
		t.Errorf("the audit row recorded trust %q; a pack result with no trust stated is recorded as third party", payload.Result.Trust)
	}
}

func TestAPackThatSaysThirdPartyIsBelieved(t *testing.T) {
	h := newHarness(true)
	server := &recordingServer{
		tools:  []capability.CapabilityID{"software.observe.fetch_prs"},
		result: environment.ActionResult{Success: true, Trust: environment.TrustThirdParty, StateDelta: map[string]any{"title": "x"}},
	}
	if err := h.gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}
	result, err := h.gw.Call(context.Background(), contractCall(), notInProcess(t))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.Trust != environment.TrustThirdParty {
		t.Errorf("trust = %q; a first-party pack saying third party is believed", result.Trust)
	}
}

func TestACallForAToolNoPackServesStaysOnTheInProcessPath(t *testing.T) {
	h := newHarness(true)
	server := &recordingServer{tools: []capability.CapabilityID{"software.observe.fetch_prs"}}
	if err := h.gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the pack: %v", err)
	}
	call := contractCall()
	call.Capability = "software.act.write_code"
	ran := 0

	result, err := h.gw.Call(context.Background(), call, func(context.Context) (environment.ActionResult, error) {
		ran++
		return environment.ActionResult{Success: true}, nil
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if ran != 1 || !result.Success {
		t.Errorf("the in-process call ran %d times with result %+v, want once and its success", ran, result)
	}
	if got := server.received(); len(got) != 0 {
		t.Errorf("the pack's server received %+v for a tool it does not serve", got)
	}
	// The in-process path keeps a result's trust as the call set it (step 1).
	if result.Trust != "" {
		t.Errorf("trust = %q; the in-process path changed the trust the call returned", result.Trust)
	}
}
