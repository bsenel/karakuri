package capability

import "strings"

// MCPDomain is the domain every capability discovered from an MCP server
// carries, and the domain the MCP environment factory registers under.
//
// It is not a domain pack and never will be. A pack declares its capabilities
// when it is written and is conformance-checked at boot; these are read off a
// server this repository did not write, at boot, and change when that server
// changes. Giving them a domain of their own is what keeps them out of every
// per-pack check without those checks needing to know MCP exists.
const MCPDomain = "mcp"

// mcpPrefix is the reserved namespace. Nothing outside MCP discovery may
// declare an ID under it — see conformance.checkReservedNamespace.
const mcpPrefix = MCPDomain + "."

// MCPCapabilityID is the ID a discovered tool is registered under:
// mcp.<instance>.<tool>.
//
// The instance is in the ID rather than only in the environment because two
// servers may both export a tool called "read_file", and a capability ID is the
// thing routing, quota, telemetry and the audit log all key on. An ID that
// named the tool alone would make one tenant's filesystem server and another's
// indistinguishable everywhere downstream.
func MCPCapabilityID(instance, tool string) CapabilityID {
	return CapabilityID(mcpPrefix + instance + "." + tool)
}

// IsMCPCapability reports whether an ID is in the reserved namespace.
//
// Four bounds are expressed by asking this, and each is at the place that would
// otherwise trust the ID: it is never valid as a Criterion.Verifier, never
// given a workspace, always approval-gated, and never declared by a pack. A
// discovered tool is a tool somebody else wrote, and the difference between it
// and a pack's capability has to survive every place the two look alike.
func IsMCPCapability(id CapabilityID) bool {
	return strings.HasPrefix(string(id), mcpPrefix)
}

// MCPInstanceOf returns the instance an MCP capability belongs to, or "" when
// the ID is not one. Used to refuse an action for a server this twin is not
// bound to, rather than silently calling whichever one happens to be resolved.
func MCPInstanceOf(id CapabilityID) string {
	if !IsMCPCapability(id) {
		return ""
	}
	rest := string(id)[len(mcpPrefix):]
	instance, _, found := strings.Cut(rest, ".")
	if !found {
		return ""
	}
	return instance
}

// MCPToolOf returns the server-side tool name an MCP capability wraps, or ""
// when the ID is not one.
//
// A server knows nothing about how this deployment namespaces what it
// discovered, so the prefix and the instance come off before tools/call names
// it. The tool name itself may contain dots, which is why this cuts once from
// the left rather than splitting on every separator.
func MCPToolOf(id CapabilityID) string {
	if !IsMCPCapability(id) {
		return ""
	}
	rest := string(id)[len(mcpPrefix):]
	_, tool, found := strings.Cut(rest, ".")
	if !found {
		return ""
	}
	return tool
}

// NewMCPCapability builds the registry entry for one discovered tool.
//
// NeedsWorkspace is absent rather than false-by-omission: a discovered tool
// never gets a git worktree, whatever its description claims about writing
// files. The loop provisions a branch for capabilities a pack declared, and a
// name that arrived over a socket this morning is not a reason to create one.
//
// Verifiable is likewise absent. A criterion settled by a tool a third party
// controls is a criterion that party can decide, which is the whole argument of
// ADR 022.
func NewMCPCapability(instance, tool, description string, input Schema) Capability {
	if input.Type == "" {
		input.Type = "object"
	}
	return Capability{
		ID:           MCPCapabilityID(instance, tool),
		Name:         tool,
		Domain:       MCPDomain,
		Description:  description,
		InputSchema:  input,
		OutputSchema: Schema{Type: "object"},
	}
}
