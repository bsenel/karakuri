package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/twin"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	featurereport "github.com/bsenel/karakuri/internal/feature/report"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// actionAuthorizer allows exactly the actions it holds, on any resource. The
// scope machinery is exercised in internal/auth; what these tests ask is which
// action each tool demands.
type actionAuthorizer map[auth.Action]bool

func (a actionAuthorizer) Authorize(_ context.Context, p auth.Principal, action auth.Action, _ auth.ResourceRef) (auth.Decision, error) {
	if a[action] {
		return auth.Decision{Allowed: true, PrincipalID: p.ID, Action: action}, nil
	}
	return auth.Decision{Allowed: false, PrincipalID: p.ID, Action: action, Reason: "no binding grants " + string(action)}, nil
}

type mcpFixture struct {
	h      *handler.MCPHandler
	denied []string // paths the OnDeny hook saw
}

func newMCPFixture(t *testing.T, allowed ...auth.Action) *mcpFixture {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "mcp.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatal(err)
	}
	store := storage.NewGORMStorage(db)
	ctx := context.Background()
	if err := store.SaveTwin(ctx, twin.DigitalTwin{ID: "twin-1", Name: "Acme", Kind: twin.KindPerson, Domain: "software"}); err != nil {
		t.Fatal(err)
	}
	objSvc := featureobjective.NewService(store)
	if _, err := objSvc.Create(ctx, featureobjective.CreateRequest{Title: "Keep the build green", Domain: "software", TwinID: "twin-1"}); err != nil {
		t.Fatal(err)
	}

	authz := actionAuthorizer{}
	for _, a := range allowed {
		authz[a] = true
	}
	f := &mcpFixture{}
	enf := auth.NewEnforcer(authz)
	enf.OnDeny = func(r *http.Request, _ auth.Principal, _ auth.Decision) { f.denied = append(f.denied, r.URL.Path) }
	f.h = &handler.MCPHandler{
		Objectives: objSvc,
		Reports:    featurereport.NewService(store, nil, nil, karakuriquota.Deps{}, featurereport.Config{}),
		Enforcer:   enf,
	}
	return f
}

// rpc posts one JSON-RPC message as an authenticated principal.
func (f *mcpFixture) rpc(t *testing.T, body string) (*httptest.ResponseRecorder, mcp.Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: "runtime-1"}))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var resp mcp.Response
	if rec.Code == http.StatusOK || rec.Code == http.StatusBadRequest {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
	}
	return rec, resp
}

func (f *mcpFixture) call(t *testing.T, tool string, args map[string]any) mcp.Response {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": mcp.MethodToolsCall,
		"params": mcp.CallToolParams{Name: tool, Arguments: args},
	})
	_, resp := f.rpc(t, string(raw))
	return resp
}

func toolResult(t *testing.T, resp mcp.Response) mcp.ToolResult {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	var res mcp.ToolResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

// The acceptance line: a client outside this process lists objectives and
// reads a digest under scopes that already exist.
func TestMCPServerListsObjectivesAndReadsADigest(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead, karakuriauth.ActionReportRead)

	res := toolResult(t, f.call(t, "objectives_list", nil))
	if res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
		t.Errorf("objectives_list = %+v", res)
	}

	res = toolResult(t, f.call(t, "digest_read", map[string]any{"twin_id": "twin-1", "window": "24h"}))
	if res.IsError {
		t.Errorf("digest_read failed: %s", res.Text())
	}
}

// ...and cannot resolve a checkpoint, however much it holds. The refusal says
// why, so a runtime does not read it as an oversight to work around.
func TestMCPServerNeverResolvesACheckpoint(t *testing.T) {
	f := newMCPFixture(t,
		karakuriauth.ActionObjectiveRead, karakuriauth.ActionReportRead, karakuriauth.ActionAuditRead,
		karakuriauth.ActionCheckpointResolve)

	for _, name := range []string{"checkpoint_resolve", "checkpoint_approve", "checkpoint_reject"} {
		resp := f.call(t, name, map[string]any{"checkpoint_id": "cp-1"})
		if resp.Error == nil || !strings.Contains(resp.Error.Message, "stays with a person") {
			t.Errorf("%s: response = %+v, want a refusal naming why", name, resp)
		}
	}

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var list mcp.ListToolsResult
	_ = json.Unmarshal(resp.Result, &list)
	for _, tool := range list.Tools {
		if strings.Contains(tool.Name, "checkpoint") {
			t.Errorf("tools/list offers %q", tool.Name)
		}
	}
}

// Each tool demands the action its REST route demands, so the list a
// principal sees is what they may call, and a call outside it is refused and
// audited under the tool's name.
func TestMCPServerFiltersToolsAndAuditsRefusals(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	if !names["objectives_list"] || !names["objective_read"] || !names["reconcile_status"] {
		t.Errorf("objective:read tools missing from %v", names)
	}
	if names["digest_read"] || names["telemetry_read"] {
		t.Errorf("tools this principal may not call were listed: %v", names)
	}

	resp = f.call(t, "digest_read", map[string]any{"twin_id": "twin-1"})
	if resp.Error == nil || resp.Error.Code != -32003 {
		t.Fatalf("digest_read without report:read = %+v, want the forbidden code", resp)
	}
	if len(f.denied) != 1 || !strings.HasSuffix(f.denied[0], "/mcp/digest_read") {
		t.Errorf("OnDeny saw %v, want one refusal named for digest_read", f.denied)
	}

	nothing := newMCPFixture(t)
	_, resp = nothing.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	_ = json.Unmarshal(resp.Result, &list)
	if len(list.Tools) != 0 {
		t.Errorf("a principal holding nothing was offered %v", list.Tools)
	}
}

func TestMCPServerProtocolEdges(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var init mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &init); err != nil || init.ProtocolVersion != mcp.ProtocolVersion {
		t.Errorf("initialize = %s (%v)", resp.Result, err)
	}
	if !strings.Contains(init.Instructions, "read-only") {
		t.Errorf("instructions do not state the boundary: %q", init.Instructions)
	}

	if rec, _ := f.rpc(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Errorf("notification answered %d %q, want 202 and no body", rec.Code, rec.Body.String())
	}
	if rec, resp := f.rpc(t, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`); rec.Code != http.StatusBadRequest || resp.Error == nil {
		t.Errorf("batch answered %d %+v, want 400 with an error", rec.Code, resp)
	}
	if _, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`); resp.Error == nil || resp.Error.Code != mcp.CodeMethodNotFound {
		t.Errorf("unknown method = %+v", resp)
	}
	if resp := f.call(t, "objective_read", nil); !toolResult(t, resp).IsError {
		t.Error("objective_read with no objective_id did not report a tool error")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no principal answered %d, want 401", rec.Code)
	}
}
