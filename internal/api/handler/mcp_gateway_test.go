package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Phase 39 step 1, served MCP surface: a tool call on /mcp takes the same one
// path a loop action takes. The gateway charges the capability quota, writes
// the call's audit row and records its cost; callTool does none of it itself.

// mcpPrincipal is the principal fixture.rpc authenticates as.
const mcpPrincipal = "runtime-1"

// gatewayBackend admits or refuses every capability take, and keeps the keys.
type gatewayBackend struct {
	allow bool
	mu    sync.Mutex
	takes []quota.Key
}

func (b *gatewayBackend) Take(_ context.Context, key quota.Key, _ quota.Policy, _ int, _ time.Time) (quota.Decision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.takes = append(b.takes, key)
	if !b.allow {
		return quota.Decision{Allowed: false, Limit: 1, Remaining: 0}, nil
	}
	return quota.Decision{Allowed: true, Limit: 1000, Remaining: 999}, nil
}

func (b *gatewayBackend) Peek(context.Context, quota.Key, quota.Policy, time.Time) (quota.Decision, error) {
	return quota.Decision{Allowed: b.allow}, nil
}

func (b *gatewayBackend) Reset(context.Context, quota.Key) error { return nil }

func (b *gatewayBackend) taken() []quota.Key {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]quota.Key(nil), b.takes...)
}

// gatewayLedger keeps every cost event the gateway records.
type gatewayLedger struct {
	mu     sync.Mutex
	events []cost.Event
}

func (l *gatewayLedger) Record(_ context.Context, e cost.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	return nil
}

func (l *gatewayLedger) Aggregate(context.Context, cost.Query) ([]cost.Bucket, error) {
	return nil, nil
}

func (l *gatewayLedger) recorded() []cost.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]cost.Event(nil), l.events...)
}

// withGateway wires a gateway over the fixture's own audit store, so the rows
// it writes are the rows auditRows reads. The handler's own Quota stays empty:
// a charge the backend sees can only have come through the gateway.
func (f *mcpFixture) withGateway(allow bool) (*gatewayBackend, *gatewayLedger) {
	backend := &gatewayBackend{allow: allow}
	ledger := &gatewayLedger{}
	costs := &karakuriquota.Recorder{Ledger: ledger}
	deps := karakuriquota.Deps{
		Backend: backend,
		TierSet: karakuriquota.NewTierResolver(karakuriquota.DefaultTiers(config.QuotaConfig{}), nil),
		Costs:   costs,
	}
	f.h.Gateway = gateway.New(f.store, deps, costs)
	return backend, ledger
}

// payloadOf decodes an audit row's payload.
func payloadOf(t *testing.T, row storage.ToolEvent) map[string]any {
	t.Helper()
	var payload map[string]any
	if row.PayloadJSON == "" {
		return payload
	}
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Fatalf("audit row %s payload is not JSON: %v: %s", row.ID, err, row.PayloadJSON)
	}
	return payload
}

// gatewayRows are the rows the gateway wrote for a served call: the ones whose
// payload names the MCP door.
func gatewayRows(t *testing.T, rows []storage.ToolEvent) []storage.ToolEvent {
	t.Helper()
	var out []storage.ToolEvent
	for _, row := range rows {
		if payloadOf(t, row)["caller"] == string(gateway.CallerMCP) {
			out = append(out, row)
		}
	}
	return out
}

// A granted tools/call is one audit row and one quota charge, both the
// gateway's: the row says the call came through MCP, by whom and for which
// tool, and the charge is keyed on the principal and the tool.
func TestMCPToolCallIsOneAuditRowAndOneQuotaChargeByTheGateway(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	backend, _ := f.withGateway(true)

	res := toolResult(t, f.call(t, "objectives_list", nil))
	if res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
		t.Fatalf("objectives_list through the gateway = %+v, want the tool's answer", res)
	}

	takes := backend.taken()
	if len(takes) != 1 {
		t.Fatalf("capability quota charges = %v, want exactly one", takes)
	}
	if key := string(takes[0]); !strings.Contains(key, mcpPrincipal) || !strings.Contains(key, "objectives_list") {
		t.Errorf("the charge was keyed %q, want it to name the principal %q and the tool", key, mcpPrincipal)
	}

	rows := f.auditRows(t)
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d (%+v), want exactly one", len(rows), rows)
	}
	row := rows[0]
	payload := payloadOf(t, row)
	if payload["caller"] != string(gateway.CallerMCP) {
		t.Errorf("audit payload caller = %v, want %q: the row is the gateway's, naming the MCP door (%s)",
			payload["caller"], gateway.CallerMCP, row.PayloadJSON)
	}
	if row.Capability != "objectives_list" {
		t.Errorf("audit row capability = %q, want the tool objectives_list", row.Capability)
	}
	if row.AgentID != mcpPrincipal && !strings.Contains(row.PayloadJSON, mcpPrincipal) {
		t.Errorf("audit row %+v does not name the principal %q", row, mcpPrincipal)
	}
	if !row.Success {
		t.Errorf("audit row records a failure for a call that succeeded: %+v", row)
	}
	if len(f.denied) != 0 {
		t.Errorf("OnDeny saw %v on a granted call, want nothing", f.denied)
	}
}

// A call the authorizer refuses never reaches the gateway: no tool runs,
// nothing is charged or costed, and the refusal is audited exactly as it is
// today, by the enforcer's deny hook under the tool's name.
func TestMCPDeniedToolCallChargesNothingAndKeepsItsDenialAudit(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	exp := &fakeExporter{}
	f.h.AuditExport = exp
	f.h.Enforcer.OnDeny = func(r *http.Request, p auth.Principal, d auth.Decision) {
		f.denied = append(f.denied, r.URL.Path)
		karakuriauth.AuditDenial(f.store)(r, p, d)
	}
	backend, ledger := f.withGateway(true)

	resp := f.call(t, "audit_export", map[string]any{"from": exportFrom, "to": exportTo})
	if resp.Error == nil || resp.Error.Code != -32003 {
		t.Fatalf("audit_export without audit:read = %+v, want the forbidden code", resp)
	}
	if exp.calls != 0 {
		t.Errorf("exporter called %d times by a refused principal, want 0", exp.calls)
	}
	if takes := backend.taken(); len(takes) != 0 {
		t.Errorf("a refused call was charged: %v", takes)
	}
	if events := ledger.recorded(); len(events) != 0 {
		t.Errorf("a refused call recorded cost: %+v", events)
	}

	if len(f.denied) != 1 || !strings.HasSuffix(f.denied[0], "/mcp/audit_export") {
		t.Errorf("OnDeny saw %v, want one refusal named for audit_export", f.denied)
	}
	rows := f.auditRows(t)
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d (%+v), want the one denial row", len(rows), rows)
	}
	if row := rows[0]; row.Kind != storage.ToolEventAuthzDenied || row.AgentID != mcpPrincipal || row.Success {
		t.Errorf("audit row = %+v, want the authorization denial for %q", row, mcpPrincipal)
	}
	if extra := gatewayRows(t, rows); len(extra) != 0 {
		t.Errorf("the gateway wrote rows for a refused call: %+v", extra)
	}
}

// A call the capability quota refuses never runs, and the client is told why
// in an MCP error rather than handed an empty answer.
func TestMCPToolCallRefusedByTheQuotaRunsNothing(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	exp := &fakeExporter{}
	f.h.AuditExport = exp
	backend, ledger := f.withGateway(false)

	resp := f.call(t, "audit_export", map[string]any{"from": exportFrom, "to": exportTo})
	if exp.calls != 0 {
		t.Errorf("exporter called %d times though the quota refused the call, want 0", exp.calls)
	}
	if resp.Error == nil {
		t.Fatalf("a call the quota refused answered %s, want an MCP error", resp.Result)
	}
	if !strings.Contains(resp.Error.Message, "quota exhausted") {
		t.Errorf("MCP error = %q, want it to say the quota is exhausted", resp.Error.Message)
	}
	if takes := backend.taken(); len(takes) != 1 {
		t.Errorf("quota takes = %v, want the one that was refused", takes)
	}
	if events := ledger.recorded(); len(events) != 0 {
		t.Errorf("a refused call recorded cost: %+v", events)
	}
	for _, row := range f.auditRows(t) {
		if row.Success {
			t.Errorf("a refused call wrote a successful audit row: %+v", row)
		}
	}
	if len(f.denied) != 0 {
		t.Errorf("OnDeny saw %v; a quota refusal is not an authorization denial", f.denied)
	}
}
