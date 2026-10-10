package mcp

import (
	"net/http"
	"testing"
)

// The era a request belongs to, as the SDK's session manager decides it (mcp
// 2.3.0, mcp/server/streamable_http_manager.py, _handle_request):
//
//	if pv is not None and pv not in HANDSHAKE_PROTOCOL_VERSIONS: -> the modern entry
//
// A 2025-06-18 streamable-HTTP client sends MCP-Protocol-Version on every
// request after the handshake, so the header being present says nothing: only a
// value outside the handshake-era revisions is a modern request. The versions
// are HANDSHAKE_PROTOCOL_VERSIONS in mcp_types/version.py, spelled out.
func TestIsModernRequestRoutesByTheHeaderValue(t *testing.T) {
	cases := []struct {
		what    string
		version string // "" sends no header
		want    bool
	}{
		{"no header", "", false},
		{"handshake-era 2024-11-05", "2024-11-05", false},
		{"handshake-era 2025-03-26", "2025-03-26", false},
		{"handshake-era 2025-06-18", "2025-06-18", false},
		{"handshake-era 2025-11-25", "2025-11-25", false},
		{"the modern revision", "2026-07-28", true},
		// Unknown values go to the modern entry, so its ladder refuses them
		// by name rather than the legacy path answering them.
		{"an unknown version", "1999-01-01", true},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			header := http.Header{}
			if tc.version != "" {
				header.Set("MCP-Protocol-Version", tc.version)
			}
			if got := IsModernRequest(header); got != tc.want {
				t.Errorf("IsModernRequest with MCP-Protocol-Version %q = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}
