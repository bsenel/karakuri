package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// The names revision 2026-07-28 puts on the wire are unexported in
// internal/platform/tools/mcp/protocol.go (metaKeyProtocolVersion,
// metaKeyClientCapabilities, resultTypeComplete, discoverResult), so they are
// written out here as the literal JSON a client sends. They carry the caveat
// protocol.go states: taken from the roadmap, not read from the specification.
// The end-to-end test below is what catches the two drifting apart.
const (
	// modernMeta is params._meta as the repository's own client sends it.
	modernMeta = `"_meta":{"protocolVersion":"2026-07-28","clientCapabilities":{}}`

	// strangerMeta names a revision this server does not speak.
	strangerMeta = `"_meta":{"protocolVersion":"2099-01-01","clientCapabilities":{}}`

	// wantResultType is resultTypeComplete.
	wantResultType = "complete"

	// sessionHeader is the header a stateful streamable-HTTP server sets and
	// neither path of this one may.
	sessionHeader = "Mcp-Session-Id"

	// forbiddenCode is the handler's codeForbidden.
	forbiddenCode = -32003
)

// modernDiscover mirrors mcp.discoverResult, field for field.
type modernDiscover struct {
	ResultType        string   `json:"resultType"`
	SupportedVersions []string `json:"supportedVersions"`
	ServerInfo        mcp.Info `json:"serverInfo"`
}

// resultFields decodes a result into its top-level fields, failing on an RPC
// error: every caller is asking what an answer carried.
func resultFields(t *testing.T, what string, resp mcp.Response) map[string]json.RawMessage {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("%s: rpc error %d: %s", what, resp.Error.Code, resp.Error.Message)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &fields); err != nil {
		t.Fatalf("%s: result %s is not an object: %v", what, resp.Result, err)
	}
	return fields
}

// wantComplete asserts a result carries resultType "complete".
func wantComplete(t *testing.T, what string, fields map[string]json.RawMessage) {
	t.Helper()
	var got string
	_ = json.Unmarshal(fields["resultType"], &got)
	if got != wantResultType {
		t.Errorf("%s: resultType = %s, want %q", what, fields["resultType"], wantResultType)
	}
}

// sameBut asserts modern is old plus a resultType and nothing else.
func sameBut(t *testing.T, what string, old, modern map[string]json.RawMessage) {
	t.Helper()
	for key, want := range old {
		if got := modern[key]; !bytes.Equal(got, want) {
			t.Errorf("%s: %s = %s on the modern path, want what the old path gives: %s", what, key, got, want)
		}
	}
	for key := range modern {
		if _, ok := old[key]; !ok && key != "resultType" {
			t.Errorf("%s: the modern path added %q", what, key)
		}
	}
}

func TestMCPServerAnswersDiscover(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var init mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &init); err != nil {
		t.Fatalf("initialize = %s (%v)", resp.Result, err)
	}

	_, resp = f.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"server/discover","params":{`+modernMeta+`}}`)
	if resp.Error != nil {
		t.Fatalf("%s: rpc error %d: %s", mcp.MethodServerDiscover, resp.Error.Code, resp.Error.Message)
	}
	var found modernDiscover
	if err := json.Unmarshal(resp.Result, &found); err != nil {
		t.Fatalf("%s = %s (%v)", mcp.MethodServerDiscover, resp.Result, err)
	}
	if found.ResultType != wantResultType {
		t.Errorf("resultType = %q, want %q", found.ResultType, wantResultType)
	}
	// Both, because the handshake stays: a client reading this list to decide
	// whether to fall back must find the revision it would fall back to.
	for _, version := range []string{mcp.ModernProtocolVersion, mcp.ProtocolVersion} {
		listed := false
		for _, got := range found.SupportedVersions {
			listed = listed || got == version
		}
		if !listed {
			t.Errorf("supportedVersions = %v, want it to contain %s", found.SupportedVersions, version)
		}
	}
	if found.ServerInfo != init.ServerInfo || found.ServerInfo.Name == "" {
		t.Errorf("serverInfo = %+v, want what initialize gives: %+v", found.ServerInfo, init.ServerInfo)
	}
}

func TestMCPServerModernResultsCarryResultType(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, oldList := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	_, newList := f.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+modernMeta+`}}`)
	fields := resultFields(t, "tools/list", newList)
	wantComplete(t, "tools/list", fields)
	sameBut(t, "tools/list", resultFields(t, "tools/list", oldList), fields)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(newList.Result, &list); err != nil || len(list.Tools) == 0 {
		t.Errorf("modern tools/list = %s (%v), want the tools this principal may call", newList.Result, err)
	}

	oldCall := f.call(t, "objectives_list", nil)
	_, newCall := f.rpc(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{},`+modernMeta+`}}`)
	fields = resultFields(t, "tools/call", newCall)
	wantComplete(t, "tools/call", fields)
	sameBut(t, "tools/call", resultFields(t, "tools/call", oldCall), fields)
	res := toolResult(t, newCall)
	if res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
		t.Errorf("modern objectives_list = %+v", res)
	}
	if got, want := res.Text(), toolResult(t, oldCall).Text(); got != want {
		t.Errorf("modern objectives_list text = %q, want the old path's %q", got, want)
	}
}

// A 2025-06-18 client sends no _meta and reads no resultType. It gets what it
// got before this revision existed.
func TestMCPServerOldPathIsUnchanged(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`)
	var init mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &init); err != nil || init.ProtocolVersion != "2025-06-18" {
		t.Errorf("initialize = %s (%v), want protocol version 2025-06-18", resp.Result, err)
	}

	_, resp = f.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	fields := resultFields(t, "tools/list", resp)
	if raw, ok := fields["resultType"]; ok {
		t.Errorf("tools/list without _meta carried resultType %s", raw)
	}
	if len(fields) != 1 || fields["tools"] == nil {
		t.Errorf("tools/list without _meta = %s, want only tools", resp.Result)
	}

	resp = f.call(t, "objectives_list", nil)
	fields = resultFields(t, "tools/call", resp)
	if raw, ok := fields["resultType"]; ok {
		t.Errorf("tools/call without _meta carried resultType %s", raw)
	}
	if len(fields) != 1 || fields["content"] == nil {
		t.Errorf("tools/call without _meta = %s, want only content", resp.Result)
	}
	if res := toolResult(t, resp); res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
		t.Errorf("objectives_list = %+v", res)
	}
}

// A revision this server does not speak is refused by name. Answering it as
// though it were understood is how a client ends up reading a result under
// rules the server never applied.
func TestMCPServerRefusesAnUnknownProtocolVersion(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	requests := map[string]string{
		mcp.MethodServerDiscover: `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + strangerMeta + `}}`,
		mcp.MethodToolsList:      `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{` + strangerMeta + `}}`,
		mcp.MethodToolsCall:      `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{},` + strangerMeta + `}}`,
	}
	for method, body := range requests {
		_, resp := f.rpc(t, body)
		if len(resp.Result) != 0 {
			t.Errorf("%s under 2099-01-01 was answered: %s", method, resp.Result)
		}
		if resp.Error == nil {
			t.Errorf("%s under 2099-01-01 carried no error", method)
			continue
		}
		if resp.Error.Code != mcp.CodeUnsupportedProtocol {
			t.Errorf("%s under 2099-01-01: error code = %d (%s), want %d",
				method, resp.Error.Code, resp.Error.Message, mcp.CodeUnsupportedProtocol)
		}
		// The shape of data is the server's to choose; what a client needs from
		// it is the versions it could have asked for.
		data, _ := json.Marshal(resp.Error.Data)
		for _, version := range []string{mcp.ModernProtocolVersion, mcp.ProtocolVersion} {
			if !strings.Contains(string(data), `"`+version+`"`) {
				t.Errorf("%s under 2099-01-01: error data = %s, want it to list %s", method, data, version)
			}
		}
	}
}

func TestMCPServerSetsNoSessionHeader(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	requests := []struct{ what, body string }{
		{"initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`},
		{"tools/list", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`},
		{"tools/call", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list"}}`},
		{"server/discover", `{"jsonrpc":"2.0","id":4,"method":"server/discover","params":{` + modernMeta + `}}`},
		{"modern tools/list", `{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{` + modernMeta + `}}`},
		{"modern tools/call", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"objectives_list",` + modernMeta + `}}`},
	}
	for _, req := range requests {
		rec, resp := f.rpc(t, req.body)
		// Answered, so this is the header of a reply and not of a refusal.
		if resp.Error != nil {
			t.Errorf("%s: rpc error %d: %s", req.what, resp.Error.Code, resp.Error.Message)
		}
		if got := rec.Header().Values(sessionHeader); len(got) != 0 {
			t.Errorf("%s set %s: %v, want no session on either path", req.what, sessionHeader, got)
		}
	}
	if rec, _ := f.rpc(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); len(rec.Header().Values(sessionHeader)) != 0 {
		t.Errorf("a notification set %s", sessionHeader)
	}
}

// The repository's own client against the repository's own server, over the
// wire. Every other test here writes the field names out by hand; this is the
// one that fails when the two ends disagree about them.
func TestMCPServerSpeaksToTheRepositoryClient(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The same injection rpc does, in place of the Authenticate middleware.
		f.h.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{ID: "runtime-1"})))
	}))
	t.Cleanup(srv.Close)

	c, err := mcp.NewClient(mcp.Config{Transport: mcp.TransportHTTP, URL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()

	got, err := c.Negotiate(ctx)
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if got.Path != mcp.PathDiscover {
		t.Errorf("Path = %q, want %q", got.Path, mcp.PathDiscover)
	}
	if got.ProtocolVersion != mcp.ModernProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", got.ProtocolVersion, mcp.ModernProtocolVersion)
	}
	if got.ServerInfo.Name != "karakuri" {
		t.Errorf("ServerInfo.Name = %q, want karakuri", got.ServerInfo.Name)
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if len(names) != 3 || !names["objectives_list"] || !names["objective_read"] || !names["reconcile_status"] {
		t.Errorf("ListTools = %v, want exactly the objective:read tools", names)
	}

	res, err := c.CallTool(ctx, "objectives_list", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
		t.Errorf("objectives_list = %+v", res)
	}
}

// The modern path is a different envelope around the same decision: the action
// a tool demands, the refusal, and the audit row do not depend on the revision.
func TestMCPServerModernPathAuthorizesAndAuditsRefusals(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "digest_read" || tool.Name == "telemetry_read" {
			t.Errorf("modern tools/list offered %q to a principal who may not call it", tool.Name)
		}
	}

	_, resp = f.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"digest_read","arguments":{"twin_id":"twin-1"},`+modernMeta+`}}`)
	if len(resp.Result) != 0 {
		t.Errorf("modern digest_read without report:read was answered: %s", resp.Result)
	}
	if resp.Error == nil || resp.Error.Code != forbiddenCode {
		t.Fatalf("modern digest_read without report:read = %+v, want the forbidden code", resp)
	}
	if len(f.denied) != 1 || !strings.HasSuffix(f.denied[0], "/mcp/digest_read") {
		t.Errorf("OnDeny saw %v, want one refusal named for digest_read", f.denied)
	}
}
