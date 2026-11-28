package capability_test

import (
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// The instance is part of the ID, so two servers exporting the same tool name
// are two capabilities everywhere downstream.
func TestMCPCapabilityIDNamesTheInstance(t *testing.T) {
	a := capability.MCPCapabilityID("acme_files", "read_file")
	b := capability.MCPCapabilityID("other_files", "read_file")
	if a == b {
		t.Fatalf("two instances' read_file share the ID %q", a)
	}
	if a != "mcp.acme_files.read_file" {
		t.Errorf("ID = %q, want mcp.acme_files.read_file", a)
	}
}

func TestIsMCPCapability(t *testing.T) {
	for _, tc := range []struct {
		id   capability.CapabilityID
		want bool
	}{
		{"mcp.acme_files.read_file", true},
		{"mcp.x", true},
		{"software.act.run_tests", false},
		// A domain that merely starts with the letters is not the namespace.
		{"mcpx.files.read", false},
		{"mcp", false},
		{"", false},
	} {
		if got := capability.IsMCPCapability(tc.id); got != tc.want {
			t.Errorf("IsMCPCapability(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// A tool name may contain dots; the instance is cut once from the left, so the
// server is asked for the name it advertised and not a fragment of it.
func TestMCPInstanceAndToolSplitOnceFromTheLeft(t *testing.T) {
	id := capability.MCPCapabilityID("acme", "fs.read.v2")
	if got := capability.MCPInstanceOf(id); got != "acme" {
		t.Errorf("instance = %q, want acme", got)
	}
	if got := capability.MCPToolOf(id); got != "fs.read.v2" {
		t.Errorf("tool = %q, want fs.read.v2", got)
	}
}

func TestMCPInstanceAndToolOfAnythingElseAreEmpty(t *testing.T) {
	for _, id := range []capability.CapabilityID{"software.act.run_tests", "mcp.onlyinstance", ""} {
		if got := capability.MCPInstanceOf(id); got != "" {
			t.Errorf("MCPInstanceOf(%q) = %q, want empty", id, got)
		}
		if got := capability.MCPToolOf(id); got != "" {
			t.Errorf("MCPToolOf(%q) = %q, want empty", id, got)
		}
	}
}

func TestNewMCPCapabilityCarriesTheServersTextInTheReservedDomain(t *testing.T) {
	c := capability.NewMCPCapability("acme", "read_file", "Reads a file.", capability.Schema{})
	if c.ID != "mcp.acme.read_file" || c.Domain != capability.MCPDomain || c.Name != "read_file" {
		t.Errorf("capability = %+v, want ID mcp.acme.read_file in domain %q named read_file", c, capability.MCPDomain)
	}
	if c.Description != "Reads a file." {
		t.Errorf("description = %q, want the server's own text", c.Description)
	}
	if c.InputSchema.Type != "object" {
		t.Errorf("input schema type = %q, want object when the server gave none", c.InputSchema.Type)
	}
	if c.Writes {
		t.Error("a discovered tool was built declaring that it writes")
	}
}
