package handler_test

import (
	"encoding/json"
	"sort"
	"testing"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// What a 2026-07-28 result carries beside its payload, read in the official
// SDK (mcp 2.3.0) on 2026-10-10:
//
//   - mcp_types/_types.py, class CacheableResult: ttl_ms (wire ttlMs, an integer
//     >= 0, default 0) and cache_scope (wire cacheScope, "public" or "private",
//     default "private"). Its docstring: "Both fields are required on the
//     2026-07-28 wire." The classes that inherit it are DiscoverResult,
//     ListToolsResult, ListResourcesResult, ListResourceTemplatesResult,
//     ReadResourceResult and ListPromptsResult. Of those this server emits two:
//     server/discover and tools/list.
//   - CallToolResult and EmptyResult inherit Result, not CacheableResult: a
//     tools/call or ping result carries neither field.
//   - mcp/server/runner.py, ServerRunner's result shaping (the method that ends
//     in `return self._stamp_server_info(version, dumped)`): a result without a
//     resultType gets "complete" on a modern version, and a per-method
//     CacheHint (mcp/server/caching.py, default ttl_ms 0; Server.cache_hints in
//     mcp/server/lowlevel/server.py) fills ttlMs and cacheScope the handler left
//     unset. With no hint the type's defaults go out: ttlMs 0, cacheScope
//     "private". Those are the values the probe server emitted.
//   - mcp/server/runner.py, ServerRunner._stamp_server_info: EVERY modern
//     result gets _meta["io.modelcontextprotocol/serverInfo"] (the constant
//     SERVER_INFO_META_KEY in mcp_types/_types.py) unless the handler set one;
//     handshake-era results are never stamped. The stamp is optional to a
//     client: mcp/client/client.py says of Client.server_info that "the
//     2026-era stamp is optional". mcp/server/_streamable_http_modern.py itself
//     names none of these fields; the shaping is the runner's.
//
// The fixtures below are the key sets of the SDK server's real answers: the
// 2026-10-10 proxy capture of mcp 2.3.0's probe server, quoted on pull request
// #170. The capture also proved the cause of the defect: the real claude CLI
// 2.1.296 discards a tools/list result without cacheScope and ttlMs, asks again
// four times and ends with no tools, and lists all ten once a proxy adds
// `"cacheScope":"private","ttlMs":0` and changes nothing else.
var (
	// {"cacheScope":"private","resultType":"complete","tools":[...],"ttlMs":0,
	//  "_meta":{"io.modelcontextprotocol/serverInfo":{"name":"probe","version":""}}}
	sdkModernListKeys = []string{"_meta", "cacheScope", "resultType", "tools", "ttlMs"}

	// {"content":[...],"isError":false,"resultType":"complete",
	//  "structuredContent":{...},"_meta":{"io.modelcontextprotocol/serverInfo":{...}}}
	sdkModernCallKeys = []string{"_meta", "content", "isError", "resultType", "structuredContent"}
)

// Keys the SDK's server sends that a client does not need, so their absence is
// logged and not failed: the same CLI accepted this server's tools/call result,
// {"content":[...],"resultType":"complete"}, which has none of them. _meta is
// the optional serverInfo stamp; isError defaults to false in CallToolResult;
// structuredContent is there only for a tool that declares an output schema.
// sameBut in mcp_modern_test.go would also have to allow them before the server
// could add any.
var sdkOptionalKeys = map[string]bool{"_meta": true, "isError": true, "structuredContent": true}

// wantSDKKeys asserts fields has every key the SDK's corresponding result has,
// the optional ones aside.
func wantSDKKeys(t *testing.T, what string, fields map[string]json.RawMessage, sdk []string) {
	t.Helper()
	got := make([]string, 0, len(fields))
	for key := range fields {
		got = append(got, key)
	}
	sort.Strings(got)
	for _, key := range sdk {
		if _, ok := fields[key]; ok {
			continue
		}
		if sdkOptionalKeys[key] {
			t.Logf("%s: no %q, which the SDK's server sends and a client does not need (keys: %v)", what, key, got)
			continue
		}
		t.Errorf("%s: no %q, which the SDK's server sends (keys: %v, the SDK's: %v)", what, key, got, sdk)
	}
}

// wantCacheHints asserts a cacheable result carries the SDK's defaults:
// cacheScope "private" and ttlMs 0, both present on the wire.
func wantCacheHints(t *testing.T, what string, fields map[string]json.RawMessage) {
	t.Helper()
	var scope string
	_ = json.Unmarshal(fields["cacheScope"], &scope)
	if scope != "private" {
		t.Errorf("%s: cacheScope = %s, want \"private\" (CacheableResult's default, what the SDK's server sent)", what, fields["cacheScope"])
	}
	var ttl *int64
	if err := json.Unmarshal(fields["ttlMs"], &ttl); err != nil || ttl == nil || *ttl != 0 {
		t.Errorf("%s: ttlMs = %s, want 0 (CacheableResult's default, what the SDK's server sent)", what, fields["ttlMs"])
	}
}

// wantNoCacheHints asserts a result carries neither cache field.
func wantNoCacheHints(t *testing.T, what string, fields map[string]json.RawMessage) {
	t.Helper()
	for _, key := range []string{"cacheScope", "ttlMs"} {
		if raw, ok := fields[key]; ok {
			t.Errorf("%s: %s = %s, want none", what, key, raw)
		}
	}
}

// The defect the real claude CLI found: a 2026-07-28 tools/list result is a
// CacheableResult and must say so.
func TestMCPServerModernListCarriesCacheHints(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	_, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""),
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)
	fields := resultFields(t, "tools/list", resp)
	wantComplete(t, "tools/list", fields)
	wantSDKKeys(t, "tools/list", fields, sdkModernListKeys)
	wantCacheHints(t, "tools/list", fields)
}

// Every other result this server emits on the modern path, against what the
// SDK's corresponding result carries.
func TestMCPServerModernResultsCarryWhatTheSDKsDo(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	// DiscoverResult is a CacheableResult as well. Its scope is this server's
	// to choose (TestMCPServerAnswersDiscover), so only presence is asked here.
	t.Run("server/discover", func(t *testing.T) {
		_, resp := f.modern(t, modernHeaders(mcp.MethodServerDiscover, ""),
			`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+modernMeta+`}}`)
		fields := resultFields(t, mcp.MethodServerDiscover, resp)
		wantSDKKeys(t, mcp.MethodServerDiscover, fields,
			[]string{"_meta", "cacheScope", "capabilities", "resultType", "supportedVersions", "ttlMs"})
	})

	// CallToolResult is not cacheable.
	t.Run("tools/call", func(t *testing.T) {
		_, resp := f.modern(t, modernHeaders(mcp.MethodToolsCall, "objectives_list"),
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"objectives_list","arguments":{},`+modernMeta+`}}`)
		fields := resultFields(t, "tools/call", resp)
		wantComplete(t, "tools/call", fields)
		wantSDKKeys(t, "tools/call", fields, sdkModernCallKeys)
		wantNoCacheHints(t, "tools/call", fields)
	})

	// EmptyResult is not cacheable either. The SDK's client no longer sends
	// ping under 2026-07-28, so there is no capture to compare with.
	t.Run("ping", func(t *testing.T) {
		_, resp := f.modern(t, modernHeaders(mcp.MethodPing, ""),
			`{"jsonrpc":"2.0","id":3,"method":"ping","params":{`+modernMeta+`}}`)
		if resp.Error != nil {
			t.Skipf("modern ping is refused (%d: %s): nothing to compare", resp.Error.Code, resp.Error.Message)
		}
		wantNoCacheHints(t, "ping", resultFields(t, "ping", resp))
	})
}

// The cache hints are 2026-07-28 vocabulary. A 2025-06-18 result does not gain
// them, whether the request named no revision or named a handshake-era one in
// its header, and it does not gain the serverInfo stamp either
// (ServerRunner._stamp_server_info: "handshake-era results are never stamped").
func TestMCPServerLegacyResultsGainNoCacheHints(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	paths := map[string]map[string]string{
		"no header":            nil,
		"a 2025-06-18 header":  {"MCP-Protocol-Version": mcp.ProtocolVersion},
		"a handshake and list": {"MCP-Protocol-Version": "2025-03-26"},
	}
	bodies := map[string]string{
		"initialize": `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`,
		"tools/list": `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		"tools/call": `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{}}}`,
		"ping":       `{"jsonrpc":"2.0","id":4,"method":"ping"}`,
	}
	for path, headers := range paths {
		for method, body := range bodies {
			what := method + " with " + path
			_, resp := f.modern(t, headers, body)
			fields := resultFields(t, what, resp)
			wantNoCacheHints(t, what, fields)
			for _, key := range []string{"resultType", "_meta"} {
				if raw, ok := fields[key]; ok {
					t.Errorf("%s: %s = %s, want none on a 2025-06-18 result", what, key, raw)
				}
			}
		}
	}
}
