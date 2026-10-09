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

// The names revision 2026-07-28 puts on the wire, written out as the literal
// JSON and headers a client sends. They are read from the official Python SDK
// (mcp 2.3.0): mcp/shared/inbound.py for the envelope keys, the routing headers
// and the validation ladder, mcp_types/jsonrpc.py for the error codes, and
// mcp/server/_streamable_http_modern.py for the HTTP status of each refusal.
const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"

	// modernMeta is params._meta as a 2026-07-28 client sends it. clientInfo
	// is optional and left out here.
	modernMeta = `"_meta":{"` + metaProtocolVersion + `":"2026-07-28","` + metaClientCapabilities + `":{}}`

	// bareMeta is the envelope under the names this server first assumed.
	bareMeta = `"_meta":{"protocolVersion":"2026-07-28","clientCapabilities":{}}`

	// strangerVersion is a revision this server does not speak.
	strangerVersion = "2099-01-01"
	strangerMeta    = `"_meta":{"` + metaProtocolVersion + `":"2099-01-01","` + metaClientCapabilities + `":{}}`

	// The SDK's HEADER_MISMATCH and UNSUPPORTED_PROTOCOL_VERSION.
	headerMismatchCode     = -32020
	unsupportedVersionCode = -32022

	// wantResultType is resultTypeComplete.
	wantResultType = "complete"

	// sessionHeader is the header a stateful streamable-HTTP server sets and
	// neither path of this one may.
	sessionHeader = "Mcp-Session-Id"

	// forbiddenCode is the handler's codeForbidden.
	forbiddenCode = -32003
)

// modernHeaders is what a 2026-07-28 client puts beside the body: the revision,
// the method and, for a name-bearing method, the name.
func modernHeaders(method, name string) map[string]string {
	h := map[string]string{"MCP-Protocol-Version": mcp.ModernProtocolVersion, "Mcp-Method": method}
	if name != "" {
		h["Mcp-Name"] = name
	}
	return h
}

// modern posts body with the given headers and decodes whatever JSON-RPC
// message came back, whichever status carried it.
func (f *mcpFixture) modern(t *testing.T, headers map[string]string, body string) (*httptest.ResponseRecorder, mcp.Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: "runtime-1"}))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var resp mcp.Response
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
	}
	return rec, resp
}

// wantRefusal asserts a request was refused with code under HTTP status 400,
// which is what the SDK's ERROR_CODE_HTTP_STATUS gives every ladder rejection.
func wantRefusal(t *testing.T, what string, rec *httptest.ResponseRecorder, resp mcp.Response, code int) {
	t.Helper()
	if len(resp.Result) != 0 {
		t.Errorf("%s was answered: %s", what, resp.Result)
	}
	if resp.Error == nil {
		t.Errorf("%s carried no error (HTTP %d)", what, rec.Code)
		return
	}
	if resp.Error.Code != code {
		t.Errorf("%s: error code = %d (%s), want %d", what, resp.Error.Code, resp.Error.Message, code)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("%s: HTTP status = %d, want %d", what, rec.Code, http.StatusBadRequest)
	}
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

// sameBut asserts modern is old plus a resultType, and the cache hints the SDK
// fills on a cacheable result, and nothing else.
func sameBut(t *testing.T, what string, old, modern map[string]json.RawMessage) {
	t.Helper()
	for key, want := range old {
		if got := modern[key]; !bytes.Equal(got, want) {
			t.Errorf("%s: %s = %s on the modern path, want what the old path gives: %s", what, key, got, want)
		}
	}
	for key := range modern {
		if _, ok := old[key]; !ok && key != "resultType" && key != "ttlMs" && key != "cacheScope" {
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

	rec, resp := f.modern(t, modernHeaders(mcp.MethodServerDiscover, ""),
		`{"jsonrpc":"2.0","id":2,"method":"server/discover","params":{`+modernMeta+`}}`)
	if rec.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
	}
	fields := resultFields(t, mcp.MethodServerDiscover, resp)
	wantComplete(t, mcp.MethodServerDiscover, fields)

	t.Run("identity is in _meta, not at the top level", func(t *testing.T) {
		if raw, ok := fields["serverInfo"]; ok {
			t.Errorf("top-level serverInfo = %s, want none: the SDK reads identity from _meta", raw)
		}
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(fields["_meta"], &meta)
		var info mcp.Info
		if err := json.Unmarshal(meta[metaServerInfo], &info); err != nil {
			t.Fatalf("_meta[%q] = %s (%v), want the server's identity", metaServerInfo, meta[metaServerInfo], err)
		}
		if info != init.ServerInfo || info.Name == "" {
			t.Errorf("_meta[%q] = %+v, want what initialize gives: %+v", metaServerInfo, info, init.ServerInfo)
		}
	})

	t.Run("supportedVersions lists the modern revision", func(t *testing.T) {
		var versions []string
		_ = json.Unmarshal(fields["supportedVersions"], &versions)
		listed := false
		for _, got := range versions {
			listed = listed || got == mcp.ModernProtocolVersion
		}
		if !listed {
			t.Errorf("supportedVersions = %s, want it to contain %s", fields["supportedVersions"], mcp.ModernProtocolVersion)
		}
	})

	t.Run("capabilities", func(t *testing.T) {
		var caps map[string]json.RawMessage
		if err := json.Unmarshal(fields["capabilities"], &caps); err != nil || caps["tools"] == nil {
			t.Errorf("capabilities = %s (%v), want an object declaring tools", fields["capabilities"], err)
		}
	})

	// mcp_types CacheableResult: cacheScope is "public" or "private", ttlMs a
	// non-negative integer, and both are always on the wire.
	t.Run("cacheScope and ttlMs", func(t *testing.T) {
		var scope string
		_ = json.Unmarshal(fields["cacheScope"], &scope)
		if scope != "public" && scope != "private" {
			t.Errorf("cacheScope = %s, want public or private", fields["cacheScope"])
		}
		var ttl *int64
		if err := json.Unmarshal(fields["ttlMs"], &ttl); err != nil || ttl == nil || *ttl < 0 {
			t.Errorf("ttlMs = %s, want a non-negative integer", fields["ttlMs"])
		}
	})
}

// The revision is chosen by the MCP-Protocol-Version header, as the SDK's
// session manager chooses it, and not by what the body says about itself.
func TestMCPServerRoutesByTheProtocolVersionHeader(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	t.Run("no header is the legacy path whatever the body carries", func(t *testing.T) {
		rec, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)
		if rec.Code != http.StatusOK {
			t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
		}
		fields := resultFields(t, "tools/list", resp)
		if raw, ok := fields["resultType"]; ok {
			t.Errorf("tools/list without the header carried resultType %s, want the 2025-06-18 result", raw)
		}
		if fields["tools"] == nil {
			t.Errorf("tools/list without the header = %s, want tools", resp.Result)
		}
	})

	t.Run("legacy initialize at 2025-06-18 still works", func(t *testing.T) {
		rec, resp := f.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`)
		var init mcp.InitializeResult
		if err := json.Unmarshal(resp.Result, &init); err != nil || resp.Error != nil || init.ProtocolVersion != "2025-06-18" {
			t.Errorf("initialize = %s, error %+v (%v), want protocol version 2025-06-18", resp.Result, resp.Error, err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("the header with an envelope is the modern path", func(t *testing.T) {
		_, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""),
			`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{`+modernMeta+`}}`)
		wantComplete(t, "tools/list", resultFields(t, "tools/list", resp))
	})

	// The body alone would have been answered as 2025-06-18. The header makes
	// it a modern request, and a modern request owes an envelope.
	t.Run("the header without an envelope is refused, not answered as legacy", func(t *testing.T) {
		rec, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""), `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)
		wantRefusal(t, "tools/list with the header and no _meta", rec, resp, mcp.CodeInvalidParams)
	})
}

// Rung 1 of the SDK's ladder: the envelope pair under its namespaced keys, or
// -32602 naming what is missing. clientInfo is optional.
func TestMCPServerRequiresTheNamespacedEnvelopeKeys(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	headers := modernHeaders(mcp.MethodToolsList, "")

	t.Run("the pair without clientInfo is accepted", func(t *testing.T) {
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)
		if rec.Code != http.StatusOK {
			t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
		}
		wantComplete(t, "tools/list", resultFields(t, "tools/list", resp))
	})

	t.Run("clientInfo beside the pair is accepted", func(t *testing.T) {
		meta := `"_meta":{"` + metaProtocolVersion + `":"2026-07-28","` + metaClientCapabilities + `":{},"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
		_, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+meta+`}}`)
		wantComplete(t, "tools/list", resultFields(t, "tools/list", resp))
	})

	t.Run("the bare names are refused, naming both missing keys", func(t *testing.T) {
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{`+bareMeta+`}}`)
		wantRefusal(t, "tools/list under the bare names", rec, resp, mcp.CodeInvalidParams)
		if resp.Error == nil {
			return
		}
		for _, key := range []string{metaProtocolVersion, metaClientCapabilities} {
			if !strings.Contains(resp.Error.Message, key) {
				t.Errorf("error message = %q, want it to name %s", resp.Error.Message, key)
			}
		}
	})

	t.Run("one missing key is named alone", func(t *testing.T) {
		meta := `"_meta":{"` + metaProtocolVersion + `":"2026-07-28"}`
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{`+meta+`}}`)
		wantRefusal(t, "tools/list without clientCapabilities", rec, resp, mcp.CodeInvalidParams)
		if resp.Error == nil {
			return
		}
		if !strings.Contains(resp.Error.Message, metaClientCapabilities) || strings.Contains(resp.Error.Message, metaProtocolVersion) {
			t.Errorf("error message = %q, want it to name only %s", resp.Error.Message, metaClientCapabilities)
		}
	})
}

// Rung 2: a client whose headers disagree with its body is told so, -32020.
func TestMCPServerRefusesHeadersThatDisagreeWithTheBody(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"objectives_list","arguments":{},` + modernMeta + `}}`

	cases := []struct {
		what    string
		headers map[string]string
		body    string
	}{
		{
			"MCP-Protocol-Version against the envelope's version",
			modernHeaders(mcp.MethodToolsList, ""),
			`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + strangerMeta + `}}`,
		},
		{
			"Mcp-Method against the JSON-RPC method",
			modernHeaders(mcp.MethodToolsList, "objectives_list"),
			call,
		},
		{
			"a missing Mcp-Method",
			map[string]string{"MCP-Protocol-Version": mcp.ModernProtocolVersion},
			`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + modernMeta + `}}`,
		},
		{
			"Mcp-Name against params.name on tools/call",
			modernHeaders(mcp.MethodToolsCall, "objective_read"),
			call,
		},
		{
			"a missing Mcp-Name on tools/call",
			modernHeaders(mcp.MethodToolsCall, ""),
			call,
		},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			rec, resp := f.modern(t, tc.headers, tc.body)
			wantRefusal(t, tc.what, rec, resp, headerMismatchCode)
		})
	}

	// decode_header_value: a name that would not survive a header travels as
	// =?base64?...?= and is compared decoded.
	t.Run("a base64 Mcp-Name that decodes to params.name agrees", func(t *testing.T) {
		// base64("objectives_list")
		_, resp := f.modern(t, modernHeaders(mcp.MethodToolsCall, "=?base64?b2JqZWN0aXZlc19saXN0?="), call)
		if resp.Error != nil {
			t.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
	})
}

func TestMCPServerModernResultsCarryResultType(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, oldList := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	_, newList := f.modern(t, modernHeaders(mcp.MethodToolsList, ""), `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+modernMeta+`}}`)
	fields := resultFields(t, "tools/list", newList)
	wantComplete(t, "tools/list", fields)
	sameBut(t, "tools/list", resultFields(t, "tools/list", oldList), fields)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(newList.Result, &list); err != nil || len(list.Tools) == 0 {
		t.Errorf("modern tools/list = %s (%v), want the tools this principal may call", newList.Result, err)
	}

	oldCall := f.call(t, "objectives_list", nil)
	_, newCall := f.modern(t, modernHeaders(mcp.MethodToolsCall, "objectives_list"), `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{},`+modernMeta+`}}`)
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

// Rung 3: a revision this server does not speak is refused by name, -32022,
// with the SDK's UnsupportedProtocolVersionErrorData. Answering it as though it
// were understood is how a client ends up reading a result under rules the
// server never applied.
func TestMCPServerRefusesAnUnknownProtocolVersion(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	requests := []struct{ method, name, body string }{
		{mcp.MethodServerDiscover, "", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + strangerMeta + `}}`},
		{mcp.MethodToolsList, "", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{` + strangerMeta + `}}`},
		{mcp.MethodToolsCall, "objectives_list", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{},` + strangerMeta + `}}`},
	}
	for _, req := range requests {
		t.Run(req.method, func(t *testing.T) {
			// Header and envelope agree, so this is not a mismatch.
			headers := modernHeaders(req.method, req.name)
			headers["MCP-Protocol-Version"] = strangerVersion
			rec, resp := f.modern(t, headers, req.body)
			wantRefusal(t, req.method+" under "+strangerVersion, rec, resp, unsupportedVersionCode)
			if resp.Error == nil {
				return
			}
			raw, _ := json.Marshal(resp.Error.Data)
			var data struct {
				Supported []string `json:"supported"`
				Requested string   `json:"requested"`
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatalf("error data = %s (%v), want supported and requested", raw, err)
			}
			if data.Requested != strangerVersion {
				t.Errorf("error data = %s, want requested %q", raw, strangerVersion)
			}
			listed := false
			for _, got := range data.Supported {
				listed = listed || got == mcp.ModernProtocolVersion
			}
			if !listed {
				t.Errorf("error data = %s, want supported to list %s", raw, mcp.ModernProtocolVersion)
			}
		})
	}
}

// The ladder's order, as the SDK runs it: a client that disagrees with itself
// is told that, rather than told the body's version is unsupported.
func TestMCPServerReportsMismatchBeforeUnsupportedVersion(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	t.Run("a supported header over an unsupported envelope", func(t *testing.T) {
		rec, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""),
			`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+strangerMeta+`}}`)
		wantRefusal(t, "tools/list", rec, resp, headerMismatchCode)
	})

	t.Run("an unsupported version with a wrong Mcp-Method", func(t *testing.T) {
		headers := modernHeaders(mcp.MethodToolsCall, "")
		headers["MCP-Protocol-Version"] = strangerVersion
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+strangerMeta+`}}`)
		wantRefusal(t, "tools/list", rec, resp, headerMismatchCode)
	})

	// And rung 1 before both: no envelope at all is -32602 whatever the headers say.
	t.Run("a missing envelope is reported before either", func(t *testing.T) {
		headers := modernHeaders(mcp.MethodToolsCall, "")
		headers["MCP-Protocol-Version"] = strangerVersion
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
		wantRefusal(t, "tools/list", rec, resp, mcp.CodeInvalidParams)
	})
}

// ERROR_CODE_HTTP_STATUS maps METHOD_NOT_FOUND to 404 on the modern path.
func TestMCPServerModernUnknownMethodIs404(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	rec, resp := f.modern(t, modernHeaders("no/such", ""), `{"jsonrpc":"2.0","id":1,"method":"no/such","params":{`+modernMeta+`}}`)
	if resp.Error == nil || resp.Error.Code != mcp.CodeMethodNotFound {
		t.Errorf("no/such = %+v, want error %d", resp, mcp.CodeMethodNotFound)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMCPServerSetsNoSessionHeader(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	requests := []struct {
		what    string
		headers map[string]string
		body    string
	}{
		{"initialize", nil, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`},
		{"tools/list", nil, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`},
		{"tools/call", nil, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list"}}`},
		{"server/discover", modernHeaders(mcp.MethodServerDiscover, ""), `{"jsonrpc":"2.0","id":4,"method":"server/discover","params":{` + modernMeta + `}}`},
		{"modern tools/list", modernHeaders(mcp.MethodToolsList, ""), `{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{` + modernMeta + `}}`},
		{"modern tools/call", modernHeaders(mcp.MethodToolsCall, "objectives_list"), `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"objectives_list",` + modernMeta + `}}`},
	}
	for _, req := range requests {
		rec, resp := f.modern(t, req.headers, req.body)
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

	_, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""), `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "digest_read" || tool.Name == "telemetry_read" {
			t.Errorf("modern tools/list offered %q to a principal who may not call it", tool.Name)
		}
	}

	_, resp = f.modern(t, modernHeaders(mcp.MethodToolsCall, "digest_read"), `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"digest_read","arguments":{"twin_id":"twin-1"},`+modernMeta+`}}`)
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
