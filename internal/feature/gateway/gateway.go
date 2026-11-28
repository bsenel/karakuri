// Package gateway is the one path every tool call takes (ADR 028, Phase 39
// step 1). The reasoning loop hands each routed action to Gateway.Call, which
// charges the capability quota, runs the call, writes its audit row and
// records its cost in one place, and returns the result with its provenance
// as the call set it.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Caller names who made a call, so the audit row can say which door it came
// through.
type Caller string

const (
	// CallerLoop is the reasoning loop's act step.
	CallerLoop Caller = "loop"
	// CallerMCP is a tool call on the served MCP surface.
	CallerMCP Caller = "mcp"
)

// quotaExhausted is the refusal text a call the capability quota stops
// carries, the words stepAct has always written.
const quotaExhausted = "capability quota exhausted for the day"

// Call is one tool call as the gateway sees it. The fields after Params are
// what only the caller knows: the gateway copies them into the audit row
// without interpreting them.
type Call struct {
	Caller      Caller
	TwinID      string
	ObjectiveID string
	AgentID     string
	// Capability is the capability or tool name the call invokes.
	Capability string
	// EnvID is the environment or adapter that runs the call; empty when
	// nothing could take it.
	EnvID string
	// Params is read after the call runs, so an entry the call adds (a
	// worktree path) reaches the audit row.
	Params map[string]any

	// LoopID names the loop in the quota log lines.
	LoopID string
	// Index is the action's position in its plan, keeping row IDs written in
	// the same nanosecond apart.
	Index int
	// RoutedBy records which rule picked the environment.
	RoutedBy   string
	Confidence float64
	// Stamp adds the caller's provenance to a row and its payload before the
	// row is saved. Nil adds nothing.
	Stamp func(row *storage.ToolEvent, payload map[string]any)

	// The call contract's fields not named above (ADR 028 section b), sent to
	// a pack's server with the call.
	TenantID string
	Pass     int
	ActionID string
	Bounds   agent.AuthorityBounds
}

// ServePack routes calls for the tools a pack's server lists to that server,
// with the call contract, instead of the in-process implementation.
//
// An acting capability (a "<domain>.act.<name>" ID) stays on the in-process
// path even when the server lists it: Phase 39 step 3 moves reads and
// verifiers only, and an act still needs the environment the loop built for
// its worktree and its bounds checks.
func (g *Gateway) ServePack(ctx context.Context, packID string, server domain.PackServer) error {
	tools, err := server.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("gateway: list the tools of pack %s: %w", packID, err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.packs == nil {
		g.packs = make(map[string]domain.PackServer)
	}
	for _, tool := range tools {
		if strings.Contains(string(tool), ".act.") {
			continue
		}
		g.packs[string(tool)] = server
	}
	return nil
}

// servedBy wraps perform so a call for a tool a pack's server serves goes to
// that server with the call contract. A result that states no trust is third
// party: the gateway cannot tell who wrote a pack's text, and taking it as the
// operator's would let it steer the loop (AGENTS.md rule 9).
func (g *Gateway) servedBy(call Call, perform Perform) Perform {
	g.mu.RLock()
	server, ok := g.packs[call.Capability]
	g.mu.RUnlock()
	if !ok {
		return perform
	}
	contract := domain.CallContract{
		TenantID:    call.TenantID,
		TwinID:      call.TwinID,
		ObjectiveID: call.ObjectiveID,
		LoopID:      call.LoopID,
		Pass:        call.Pass,
		ActionID:    call.ActionID,
		Bounds:      call.Bounds,
	}
	return func(ctx context.Context) (environment.ActionResult, error) {
		result, err := server.CallTool(ctx, contract, capability.CapabilityID(call.Capability), call.Params)
		if result.Trust == "" {
			result.Trust = environment.TrustThirdParty
		}
		return result, err
	}
}

// Perform is the work behind a call: the environment's Act, or the served
// tool's implementation.
type Perform func(ctx context.Context) (environment.ActionResult, error)

// QuotaRefusal is the error a call the capability quota stopped returns. It
// carries the decision so the caller can report the pressure it saw.
type QuotaRefusal struct {
	Decision quota.Decision
}

func (*QuotaRefusal) Error() string { return quotaExhausted }

// Gateway applies quota, audit and cost to every tool call.
type Gateway struct {
	store storage.StorageAdapter
	quota karakuriquota.Deps
	costs *karakuriquota.Recorder

	// packs maps a tool to the pack server that serves it (Phase 39 step 3).
	mu    sync.RWMutex
	packs map[string]domain.PackServer
}

// New builds a gateway over the audit store, the quota dependencies and the
// cost recorder.
func New(store storage.StorageAdapter, quota karakuriquota.Deps, costs *karakuriquota.Recorder) *Gateway {
	return &Gateway{store: store, quota: quota, costs: costs}
}

// Call runs one tool call through the gateway.
//
// A call the capability quota refuses never runs, writes no row and records
// no cost; its result says why and its error is a *QuotaRefusal. Every other
// call, including one perform reports as failed, writes exactly one audit row
// (and a bounds row when the result carries a violation) and one cost entry.
func (g *Gateway) Call(ctx context.Context, call Call, perform Perform) (environment.ActionResult, error) {
	// Before the call rather than after, because the point of the tier is to
	// stop a misconfigured watcher hammering an external service: most
	// capabilities reach GitHub or Linear, and a charge levied afterwards
	// would have already made the call.
	if dec, ok := g.allowCapability(ctx, call); !ok {
		return environment.ActionResult{Success: false, Error: quotaExhausted}, &QuotaRefusal{Decision: dec}
	}

	result, err := g.servedBy(call, perform)(ctx)
	if err != nil {
		// What the call reported beside its error stays: a coding agent
		// refused for quota still names the salvage branch its edits went to
		// (Phase 40 step 8).
		result.Success = false
		if result.Error == "" {
			result.Error = err.Error()
		}
	}

	g.audit(ctx, call, result)

	// Attributed to the objective rather than the twin, because "which piece
	// of work spent this" is the question a bill prompts; the twin is still
	// the subject that pays. A call made for no objective (a served MCP read)
	// leaves the type empty, which the recorder attributes to the subject.
	resourceType := "objective"
	if call.ObjectiveID == "" {
		resourceType = ""
	}
	g.costs.Record(ctx, karakuriquota.Spend{
		TwinID:       call.TwinID,
		ResourceType: resourceType,
		ResourceID:   call.ObjectiveID,
		Provider:     call.EnvID,
		Units:        1,
		UnitKind:     cost.UnitCalls,
	})

	return result, err
}

// allowCapability charges the twin's daily allowance for one capability and
// reports whether the call may run, with the decision behind a refusal.
//
// It fails open, like every other limiter here: a counter that cannot be read
// must not stop work that is otherwise fine, because a quota backend outage
// turning into a halted loop does more damage than the calls it would have
// bounded.
func (g *Gateway) allowCapability(ctx context.Context, call Call) (quota.Decision, bool) {
	if g.quota.Backend == nil || call.TwinID == "" || call.Capability == "" {
		return quota.Decision{}, true
	}
	dec, err := g.quota.TakeCapability(ctx, call.TwinID, call.Capability, time.Now())
	if err != nil {
		slog.Error("capability quota could not be read; allowing the action",
			"twin", call.TwinID, "capability", call.Capability, "loop", call.LoopID, "err", err)
		return dec, true
	}
	if dec.Allowed {
		return dec, true
	}
	slog.Warn("capability quota exhausted; skipping the action",
		"twin", call.TwinID, "capability", call.Capability, "loop", call.LoopID,
		"limit", dec.Limit, "reset_at", dec.ResetAt)
	return dec, false
}

// audit writes the call's row and, when the result carries one, its bounds
// violation row of the refusal kind (Phase 40 step 3).
func (g *Gateway) audit(ctx context.Context, call Call, result environment.ActionResult) {
	// routed_by records which rule picked the environment, because "the pack
	// said so" and "the model said so" are different claims about the same
	// successful action.
	payload := map[string]any{
		"caller": string(call.Caller),
		"params": call.Params,
		"result": result,
	}
	if call.RoutedBy != "" {
		payload["routed_by"] = call.RoutedBy
	}
	row := storage.ToolEvent{
		ID:          fmt.Sprintf("te-%d-%d", time.Now().UnixNano(), call.Index),
		ObjectiveID: call.ObjectiveID,
		AgentID:     call.AgentID,
		Capability:  call.Capability,
		Adapter:     call.EnvID,
		Success:     result.Success,
		Confidence:  call.Confidence,
		CreatedAt:   time.Now().UTC(),
	}
	g.save(ctx, call, row, payload)

	v := result.BoundsViolation
	if v == nil {
		return
	}
	g.save(ctx, call, storage.ToolEvent{
		ID:               fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), call.Index),
		ObjectiveID:      call.ObjectiveID,
		AgentID:          call.AgentID,
		Capability:       call.Capability,
		Adapter:          call.EnvID,
		Success:          false,
		Confidence:       call.Confidence,
		Kind:             storage.ToolEventRefusal,
		EscalationReason: v.Reason,
		BoundsViolation:  true,
		CreatedAt:        time.Now().UTC(),
	}, map[string]any{
		"capability": call.Capability,
		"env_id":     call.EnvID,
		"refs":       v.Refs,
	})
}

// save stamps the caller's provenance on a row and saves it. Best effort, as
// the loop's rows always were: a failed audit write does not undo the call.
func (g *Gateway) save(ctx context.Context, call Call, row storage.ToolEvent, payload map[string]any) {
	if call.Stamp != nil {
		call.Stamp(&row, payload)
	}
	payloadJSON, _ := json.Marshal(payload)
	row.PayloadJSON = string(payloadJSON)
	_ = g.store.SaveToolEvent(ctx, row)
}
