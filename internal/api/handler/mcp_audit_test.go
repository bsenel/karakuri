package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/feature/audit"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// seedAudit stores rows in the fixture's audit log.
func (f *mcpFixture) seedAudit(t *testing.T, events ...storage.ToolEvent) {
	t.Helper()
	for _, e := range events {
		if err := f.store.SaveToolEvent(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
}

// auditRows is every row in the fixture's audit log, read past the handler.
func (f *mcpFixture) auditRows(t *testing.T) []storage.ToolEvent {
	t.Helper()
	rows, err := f.store.ListToolEvents(context.Background(), storage.ToolEventFilter{OldestFirst: true})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func (f *mcpFixture) toolNames(t *testing.T) map[string]bool {
	t.Helper()
	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	return names
}

// auditListIDs calls audit_list and returns the IDs it listed, sorted.
func auditListIDs(t *testing.T, f *mcpFixture, args map[string]any) []string {
	t.Helper()
	res := toolResult(t, f.call(t, "audit_list", args))
	if res.IsError {
		t.Fatalf("audit_list(%v) failed: %s", args, res.Text())
	}
	var rows []storage.ToolEvent
	if err := json.Unmarshal([]byte(res.Text()), &rows); err != nil {
		t.Fatalf("audit_list result is not a JSON array of audit rows: %v: %s", err, res.Text())
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	slices.Sort(ids)
	return ids
}

// assertAuditFilter stores two rows that differ only in one field and expects
// args to list the first alone. A filter the tool drops lists both.
func assertAuditFilter(t *testing.T, args map[string]any, want, other storage.ToolEvent) {
	t.Helper()
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	want.ID, other.ID = "ev-want", "ev-other"
	f.seedAudit(t, want, other)

	if ids := auditListIDs(t, f, nil); !slices.Equal(ids, []string{"ev-other", "ev-want"}) {
		t.Fatalf("unfiltered audit_list = %v, want both rows", ids)
	}
	if ids := auditListIDs(t, f, args); !slices.Equal(ids, []string{"ev-want"}) {
		t.Errorf("audit_list(%v) = %v, want [ev-want]", args, ids)
	}
}

// The audit tools demand audit:read, the action GET /audit and
// GET /audit/export demand: listed with it, and without it neither listed nor
// callable, the refusal audited under the tool's name.
func TestMCPAuditToolsRequireAuditRead(t *testing.T) {
	granted := newMCPFixture(t, karakuriauth.ActionAuditRead)
	if names := granted.toolNames(t); !names["audit_list"] || !names["audit_export"] {
		t.Errorf("audit:read tools missing from %v", names)
	}

	// Everything the other tools demand, and not audit:read.
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead, karakuriauth.ActionReportRead)
	exp := &fakeExporter{}
	f.h.AuditExport = exp
	f.seedAudit(t, storage.ToolEvent{ID: "ev-1", ObjectiveID: "obj-1"})

	if names := f.toolNames(t); names["audit_list"] || names["audit_export"] {
		t.Errorf("tools this principal may not call were listed: %v", names)
	}

	calls := []struct {
		tool string
		args map[string]any
	}{
		{"audit_list", nil},
		{"audit_export", map[string]any{"from": exportFrom, "to": exportTo}},
	}
	for i, c := range calls {
		resp := f.call(t, c.tool, c.args)
		if resp.Error == nil || resp.Error.Code != -32003 {
			t.Fatalf("%s without audit:read = %+v, want the forbidden code", c.tool, resp)
		}
		// The decision's own reason, which is what callTool answers with.
		if want := "no binding grants " + string(karakuriauth.ActionAuditRead); resp.Error.Message != want {
			t.Errorf("%s refusal = %q, want %q", c.tool, resp.Error.Message, want)
		}
		if len(f.denied) != i+1 || !strings.HasSuffix(f.denied[i], "/mcp/"+c.tool) {
			t.Errorf("OnDeny saw %v, want refusal %d named for %s", f.denied, i+1, c.tool)
		}
	}
	if exp.calls != 0 {
		t.Errorf("exporter called %d times by a refused principal, want 0", exp.calls)
	}
}

func TestMCPAuditListReturnsStoredEvents(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.seedAudit(t, storage.ToolEvent{
		ID: "ev-a", ObjectiveID: "obj-1", AgentID: "agent-1", Kind: storage.ToolEventEscalation,
		EscalationReason: "outside bounds", BoundsViolation: true,
		Provider: "anthropic", Model: "model-a", TemplateID: "tmpl-green-build", AutonomyRung: "propose",
	})

	res := toolResult(t, f.call(t, "audit_list", nil))
	if res.IsError {
		t.Fatalf("audit_list failed: %s", res.Text())
	}
	// The wire shape GET /audit answers with, field names included.
	var rows []map[string]any
	if err := json.Unmarshal([]byte(res.Text()), &rows); err != nil {
		t.Fatalf("audit_list result is not a JSON array: %v: %s", err, res.Text())
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %s", len(rows), res.Text())
	}
	for field, want := range map[string]any{
		"id":                "ev-a",
		"objective_id":      "obj-1",
		"agent_id":          "agent-1",
		"kind":              storage.ToolEventEscalation,
		"escalation_reason": "outside bounds",
		"bounds_violation":  true,
		"provider":          "anthropic",
		"model":             "model-a",
		"template_id":       "tmpl-green-build",
		"autonomy_rung":     "propose",
	} {
		if got := rows[0][field]; got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

// An empty log is an empty array, as it is on the REST route, not a tool error.
func TestMCPAuditListOfAnEmptyLogIsAnEmptyArray(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	res := toolResult(t, f.call(t, "audit_list", nil))
	if res.IsError || strings.TrimSpace(res.Text()) != "[]" {
		t.Errorf("audit_list on an empty log = %+v, want []", res)
	}
}

func TestMCPAuditListFiltersByObjective(t *testing.T) {
	assertAuditFilter(t, map[string]any{"objective_id": "obj-1"},
		storage.ToolEvent{ObjectiveID: "obj-1"}, storage.ToolEvent{ObjectiveID: "obj-2"})
}

func TestMCPAuditListFiltersByAgent(t *testing.T) {
	assertAuditFilter(t, map[string]any{"agent_id": "agent-1"},
		storage.ToolEvent{ObjectiveID: "obj-1", AgentID: "agent-1"},
		storage.ToolEvent{ObjectiveID: "obj-1", AgentID: "agent-2"})
}

func TestMCPAuditListFiltersByKind(t *testing.T) {
	assertAuditFilter(t, map[string]any{"kind": storage.ToolEventEscalation},
		storage.ToolEvent{ObjectiveID: "obj-1", Kind: storage.ToolEventEscalation},
		storage.ToolEvent{ObjectiveID: "obj-1", Kind: storage.ToolEventExecute})
}

func TestMCPAuditListFiltersByProvider(t *testing.T) {
	assertAuditFilter(t, map[string]any{"provider": "anthropic"},
		storage.ToolEvent{ObjectiveID: "obj-1", Provider: "anthropic"},
		storage.ToolEvent{ObjectiveID: "obj-1", Provider: "fallback"})
}

func TestMCPAuditListFiltersByModel(t *testing.T) {
	assertAuditFilter(t, map[string]any{"model": "model-a"},
		storage.ToolEvent{ObjectiveID: "obj-1", Model: "model-a"},
		storage.ToolEvent{ObjectiveID: "obj-1", Model: "model-b"})
}

// Named template, as the REST query parameter is, though the row's field is
// template_id.
func TestMCPAuditListFiltersByTemplate(t *testing.T) {
	assertAuditFilter(t, map[string]any{"template": "tmpl-triage"},
		storage.ToolEvent{ObjectiveID: "obj-1", TemplateID: "tmpl-triage"},
		storage.ToolEvent{ObjectiveID: "obj-1", TemplateID: "tmpl-green-build"})
}

// Tri-state, as on the REST route: absent lists both, true only violations,
// false only clean rows.
func TestMCPAuditListFiltersByBoundsViolation(t *testing.T) {
	violation := storage.ToolEvent{ObjectiveID: "obj-1", BoundsViolation: true}
	clean := storage.ToolEvent{ObjectiveID: "obj-1"}
	t.Run("true", func(t *testing.T) {
		assertAuditFilter(t, map[string]any{"bounds_violation": true}, violation, clean)
	})
	t.Run("false", func(t *testing.T) {
		assertAuditFilter(t, map[string]any{"bounds_violation": false}, clean, violation)
	})
}

// since is an inclusive lower bound on when a row was written. The store
// stamps rows itself, so the two rows here are one row asked about twice.
func TestMCPAuditListFiltersBySince(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.seedAudit(t, storage.ToolEvent{ID: "ev-now", ObjectiveID: "obj-1"})

	earlier := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	later := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if ids := auditListIDs(t, f, map[string]any{"since": earlier}); !slices.Equal(ids, []string{"ev-now"}) {
		t.Errorf("audit_list since an hour ago = %v, want [ev-now]", ids)
	}
	if ids := auditListIDs(t, f, map[string]any{"since": later}); len(ids) != 0 {
		t.Errorf("audit_list since an hour from now = %v, want nothing", ids)
	}
}

func TestMCPAuditListAppliesLimit(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.seedAudit(t,
		storage.ToolEvent{ID: "ev-1", ObjectiveID: "obj-1"},
		storage.ToolEvent{ID: "ev-2", ObjectiveID: "obj-1"},
		storage.ToolEvent{ID: "ev-3", ObjectiveID: "obj-1"})

	if ids := auditListIDs(t, f, map[string]any{"limit": 2}); len(ids) != 2 {
		t.Errorf("audit_list limit 2 = %v, want two rows", ids)
	}
}

// With no limit the REST route lists a hundred rows, and so does the tool: an
// uncapped read of the whole log is not what an absent argument asks for.
func TestMCPAuditListDefaultsToAHundredRows(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	for i := range 101 {
		f.seedAudit(t, storage.ToolEvent{ID: fmt.Sprintf("ev-%03d", i), ObjectiveID: "obj-1"})
	}
	if ids := auditListIDs(t, f, nil); len(ids) != 100 {
		t.Errorf("audit_list with no limit listed %d rows, want 100", len(ids))
	}
}

// The tool's text is the exporter's document for the same closed window, byte
// for byte — what GET /audit/export writes, so a digest taken over either is a
// digest of both.
func TestMCPAuditExportReturnsTheExportersDocument(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.seedAudit(t,
		storage.ToolEvent{ID: "ev-decision", ObjectiveID: "obj-1", Kind: storage.ToolEventEscalation, Provider: "anthropic", Model: "model-a"},
		storage.ToolEvent{ID: "ev-oversight", ObjectiveID: "obj-1", Kind: storage.ToolEventApproval, Approver: "ada"})

	from := time.Now().Add(-time.Hour).UTC()
	to := time.Now().UTC() // after both rows, and already past: a closed window
	exporter := audit.NewExporter(f.store, audit.Retention{FloorDays: audit.FloorDays}, nil)
	want, err := exporter.Export(context.Background(), from, to, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(want), "ev-decision") || !strings.Contains(string(want), "ev-oversight") {
		t.Fatalf("the window does not cover the seeded rows: %s", want)
	}

	res := toolResult(t, f.call(t, "audit_export", map[string]any{
		"from": from.Format(time.RFC3339Nano), "to": to.Format(time.RFC3339Nano),
	}))
	if res.IsError {
		t.Fatalf("audit_export failed: %s", res.Text())
	}
	if got := res.Text(); got != string(want) {
		t.Errorf("audit_export = %s\nwant the exporter's bytes: %s", got, want)
	}
}

// The tool assembles nothing: it parses the bounds, hands them to the exporter
// with the time of the call, and returns what comes back untouched.
func TestMCPAuditExportPassesTheWindowToTheExporter(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	exp := &fakeExporter{}
	f.h.AuditExport = exp

	before := time.Now()
	res := toolResult(t, f.call(t, "audit_export", map[string]any{"from": exportFrom, "to": exportTo}))
	after := time.Now()

	if res.IsError {
		t.Fatalf("audit_export failed: %s", res.Text())
	}
	if got := res.Text(); got != exportBytes {
		t.Errorf("audit_export = %q, want %q byte for byte", got, exportBytes)
	}
	if exp.calls != 1 {
		t.Fatalf("exporter called %d times, want 1", exp.calls)
	}
	wantFrom, _ := time.Parse(time.RFC3339, exportFrom)
	wantTo, _ := time.Parse(time.RFC3339, exportTo)
	if !exp.from.Equal(wantFrom) || !exp.to.Equal(wantTo) {
		t.Errorf("exporter got [%s, %s), want [%s, %s)", exp.from, exp.to, wantFrom, wantTo)
	}
	if exp.now.Before(before) || exp.now.After(after) {
		t.Errorf("exporter got now = %s, want the time of the call", exp.now)
	}
}

// A missing or malformed bound is refused before the exporter is asked, where
// ExportWindow answers 400, and with what it says. It is a tool result: the
// caller asked a tool it may call and got a bad answer.
func TestMCPAuditExportRefusesABadBound(t *testing.T) {
	const fromMessage, toMessage = "from must be an RFC3339 timestamp", "to must be an RFC3339 timestamp"
	cases := map[string]struct {
		args map[string]any
		want string
	}{
		"missing from":     {map[string]any{"to": exportTo}, fromMessage},
		"missing to":       {map[string]any{"from": exportFrom}, toMessage},
		"unparseable from": {map[string]any{"from": "last tuesday", "to": exportTo}, fromMessage},
		"unparseable to":   {map[string]any{"from": exportFrom, "to": "2026-02-01"}, toMessage},
		"no arguments":     {nil, fromMessage},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMCPFixture(t, karakuriauth.ActionAuditRead)
			exp := &fakeExporter{}
			f.h.AuditExport = exp

			res := toolResult(t, f.call(t, "audit_export", tc.args))
			if !res.IsError {
				t.Fatalf("audit_export(%v) = %q, want a tool error", tc.args, res.Text())
			}
			if !strings.Contains(res.Text(), tc.want) {
				t.Errorf("tool error = %q, want it to say %q", res.Text(), tc.want)
			}
			if exp.calls != 0 {
				t.Errorf("exporter called %d times, want 0", exp.calls)
			}
		})
	}
}

// An open window, and one that ends before it starts, are refused by the
// exporter itself, and the tool error carries its reason as the REST body does.
func TestMCPAuditExportRefusesAnOpenOrInvertedWindow(t *testing.T) {
	now := time.Now().UTC()
	cases := map[string]struct {
		from, to time.Time
		want     string
	}{
		"open":     {now.Add(-time.Hour), now.Add(time.Hour), "a window must have ended to be exported"},
		"inverted": {now.Add(-time.Hour), now.Add(-2 * time.Hour), "is not before to"},
		"empty":    {now.Add(-time.Hour), now.Add(-time.Hour), "is not before to"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMCPFixture(t, karakuriauth.ActionAuditRead)
			res := toolResult(t, f.call(t, "audit_export", map[string]any{
				"from": tc.from.Format(time.RFC3339), "to": tc.to.Format(time.RFC3339),
			}))
			if !res.IsError {
				t.Fatalf("audit_export of an %s window = %q, want a tool error", name, res.Text())
			}
			if text := res.Text(); !strings.Contains(text, audit.ErrWindow.Error()) || !strings.Contains(text, tc.want) {
				t.Errorf("tool error = %q, want %q and %q", text, audit.ErrWindow.Error(), tc.want)
			}
		})
	}
}

// An exporter that fails for its own reasons is a tool error too: the tool
// ran and failed, which is the one thing a tool result can say and a protocol
// error cannot.
func TestMCPAuditExportReportsAnExporterFailureAsAToolError(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.h.AuditExport = &fakeExporter{err: errors.New("database is closed")}

	res := toolResult(t, f.call(t, "audit_export", map[string]any{"from": exportFrom, "to": exportTo}))
	if !res.IsError || !strings.Contains(res.Text(), "database is closed") {
		t.Errorf("audit_export = %+v, want a tool error carrying the exporter's", res)
	}
}

// Reading the audit log is not an audited act: a granted call leaves the log
// as it found it, and reaches no deny hook.
func TestMCPAuditToolsWriteNothing(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionAuditRead)
	f.seedAudit(t,
		storage.ToolEvent{ID: "ev-1", ObjectiveID: "obj-1", Kind: storage.ToolEventExecute},
		storage.ToolEvent{ID: "ev-2", ObjectiveID: "obj-1", Kind: storage.ToolEventApproval})
	before := f.auditRows(t)

	closed := time.Now().UTC().Format(time.RFC3339Nano)
	calls := []struct {
		tool string
		args map[string]any
	}{
		{"audit_list", nil},
		{"audit_list", map[string]any{"kind": storage.ToolEventApproval, "limit": 1}},
		{"audit_export", map[string]any{"from": exportFrom, "to": closed}},
	}
	for _, c := range calls {
		if res := toolResult(t, f.call(t, c.tool, c.args)); res.IsError {
			t.Fatalf("%s(%v) failed: %s", c.tool, c.args, res.Text())
		}
	}

	if after := f.auditRows(t); !slices.Equal(after, before) {
		t.Errorf("the audit log changed under a read:\nbefore %+v\nafter  %+v", before, after)
	}
	if len(f.denied) != 0 {
		t.Errorf("OnDeny saw %v on granted calls, want nothing", f.denied)
	}
}
