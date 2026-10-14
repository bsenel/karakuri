package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// handshakeVersions is HANDSHAKE_PROTOCOL_VERSIONS in the SDK's
// mcp_types/version.py (mcp 2.3.0): the revisions reached through `initialize`.
// A streamable-HTTP client of any of them names it in MCP-Protocol-Version on
// every request after the handshake, and the SDK's session manager
// (mcp/server/streamable_http_manager.py, _handle_request) keeps such a request
// on the legacy path. This server's legacy path does not look at the header, so
// it owes all four the same answer.
var handshakeVersions = []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}

// A real 2025-06-18 client, Claude Code among them, initializes and then sends
// the header with every request. It is still a legacy client: no envelope, no
// resultType, HTTP 200.
func TestMCPServerAnswersAHandshakeEraHeaderOnTheLegacyPath(t *testing.T) {
	for _, version := range handshakeVersions {
		t.Run(version, func(t *testing.T) {
			f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
			headers := map[string]string{"MCP-Protocol-Version": version}

			_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`)
			var init mcp.InitializeResult
			if err := json.Unmarshal(resp.Result, &init); err != nil || resp.Error != nil || init.ProtocolVersion != "2025-06-18" {
				t.Fatalf("initialize = %s, error %+v (%v), want protocol version 2025-06-18", resp.Result, resp.Error, err)
			}

			t.Run("tools/list", func(t *testing.T) {
				rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
				if rec.Code != http.StatusOK {
					t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
				}
				fields := resultFields(t, "tools/list with the header "+version, resp)
				if raw, ok := fields["resultType"]; ok {
					t.Errorf("tools/list carried resultType %s, want the legacy result", raw)
				}
				var list mcp.ListToolsResult
				if err := json.Unmarshal(resp.Result, &list); err != nil || len(list.Tools) == 0 {
					t.Fatalf("tools/list = %s (%v), want the tools this principal may call", resp.Result, err)
				}
				names := map[string]bool{}
				for _, tool := range list.Tools {
					names[tool.Name] = true
				}
				if !names["objectives_list"] {
					t.Errorf("tools/list = %v, want objectives_list among them", names)
				}
			})

			t.Run("tools/call", func(t *testing.T) {
				rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"objectives_list","arguments":{}}}`)
				if rec.Code != http.StatusOK {
					t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
				}
				fields := resultFields(t, "tools/call with the header "+version, resp)
				if raw, ok := fields["resultType"]; ok {
					t.Errorf("tools/call carried resultType %s, want the legacy result", raw)
				}
				if res := toolResult(t, resp); res.IsError || !strings.Contains(res.Text(), "Keep the build green") {
					t.Errorf("objectives_list = %+v", res)
				}
			})
		})
	}
}

// The other side of the same rule: a header naming anything that is not a
// handshake-era revision goes down the modern ladder, as in the SDK.
func TestMCPServerSendsOtherHeaderVersionsDownTheModernLadder(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)

	t.Run("2026-07-28 without an envelope is refused", func(t *testing.T) {
		rec, resp := f.modern(t, map[string]string{"MCP-Protocol-Version": "2026-07-28"},
			`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		wantRefusal(t, "tools/list at 2026-07-28 with no _meta", rec, resp, mcp.CodeInvalidParams)
	})

	t.Run("2026-07-28 with its envelope is answered as modern", func(t *testing.T) {
		rec, resp := f.modern(t, modernHeaders(mcp.MethodToolsList, ""),
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+modernMeta+`}}`)
		if rec.Code != http.StatusOK {
			t.Errorf("HTTP status = %d, want %d", rec.Code, http.StatusOK)
		}
		wantComplete(t, "tools/list", resultFields(t, "tools/list", resp))
	})

	// 1999-01-01 is no revision at all. The legacy path must not answer it:
	// the modern ladder refuses it by name.
	t.Run("an unknown version is refused as unsupported", func(t *testing.T) {
		const unknown = "1999-01-01"
		meta := `"_meta":{"` + metaProtocolVersion + `":"` + unknown + `","` + metaClientCapabilities + `":{}}`
		headers := modernHeaders(mcp.MethodToolsList, "")
		headers["MCP-Protocol-Version"] = unknown
		rec, resp := f.modern(t, headers, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{`+meta+`}}`)
		wantRefusal(t, "tools/list under "+unknown, rec, resp, unsupportedVersionCode)
	})

	t.Run("an unknown version without an envelope is not answered as legacy", func(t *testing.T) {
		rec, resp := f.modern(t, map[string]string{"MCP-Protocol-Version": "1999-01-01"},
			`{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)
		wantRefusal(t, "tools/list under 1999-01-01 with no _meta", rec, resp, mcp.CodeInvalidParams)
	})
}
