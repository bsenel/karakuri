package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// fsServer is the smallest MCP server over streamable HTTP: it advertises a
// filesystem server's tools and records the Authorization it was sent.
func fsServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		auth = r.Header.Get("Authorization")
		if req.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case mcp.MethodInitialize:
			result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion, ServerInfo: mcp.Info{Name: "fs"}}
		case mcp.MethodToolsList:
			result = mcp.ListToolsResult{Tools: []mcp.Tool{
				{Name: "read_file", Description: "Reads a file."},
				{Name: "list_directory", Description: "Lists a directory."},
				{Name: "write_file", Description: "Writes a file."},
			}}
		}
		raw, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: raw})
	}))
	t.Cleanup(srv.Close)
	return srv, &auth
}

// The acceptance line, at the registry: a filesystem server configured as an
// instance appears in /health with its tools — as the row every slot has, and
// with the detail only a server has.
func TestMCPInstanceAppearsInHealthWithItsTools(t *testing.T) {
	srv, auth := fsServer(t)
	r := NewRegistryFromConfig(config.ToolsConfig{MCP: config.SlotConfig{
		Default: "acme_files",
		Instances: map[string]config.InstanceConfig{
			"acme_files": {Type: "streamable_http", Options: map[string]any{
				"url":           srv.URL,
				"bearer_token":  "s3cret",
				"allowed_tools": []any{"read_file", "list_directory"},
			}},
			"typo": {Type: "smoke_signals"},
		},
	}})
	t.Cleanup(r.CloseMCP)

	health := r.MCPHealth()
	if len(health) != 1 {
		t.Fatalf("health = %+v, want one instance (an unknown type is skipped)", health)
	}
	h := health[0]
	if h.Name != "acme_files" || h.State != mcp.StateConnected || !h.IsDefault {
		t.Errorf("health = %+v", h)
	}
	if want := []string{"list_directory", "read_file"}; !reflect.DeepEqual(h.Tools, want) {
		t.Errorf("tools = %v, want %v", h.Tools, want)
	}
	if want := []string{"write_file"}; !reflect.DeepEqual(h.Filtered, want) {
		t.Errorf("filtered = %v, want %v", h.Filtered, want)
	}
	if *auth != "Bearer s3cret" {
		t.Errorf("Authorization = %q, want the bearer_token as a header", *auth)
	}

	var row *AdapterStatus
	for _, s := range r.Status() {
		if s.Slot == "mcp" {
			s := s
			row = &s
		}
	}
	if row == nil || row.Instance != "acme_files" || !row.Active || !row.IsDefault {
		t.Errorf("mcp status row = %+v, want acme_files active and default", row)
	}

	caps := r.MCPCapabilities()
	if len(caps) != 2 || caps[0].ID != capability.MCPCapabilityID("acme_files", "list_directory") {
		t.Errorf("capabilities = %+v", caps)
	}
	facs := r.MCPEnvironmentFactories()
	if len(facs) != 1 || len(facs[0].Serves) != 2 || facs[0].Domain != capability.MCPDomain {
		t.Errorf("factories = %+v", facs)
	}
}

func TestBearerHeadersKeepsExistingHeaders(t *testing.T) {
	got := bearerHeaders(map[string]string{"X-Tenant": "acme"}, "t")
	if got["X-Tenant"] != "acme" || got["Authorization"] != "Bearer t" {
		t.Errorf("headers = %v", got)
	}
	in := map[string]string{"X-Tenant": "acme"}
	if got := bearerHeaders(in, ""); !reflect.DeepEqual(got, in) {
		t.Errorf("no token changed the headers: %v", got)
	}
}
