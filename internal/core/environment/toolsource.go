package environment

import "github.com/bsenel/karakuri/internal/core/capability"

// ToolSource is implemented by an environment whose capabilities were
// discovered rather than declared by a pack.
//
// It exists for one reader: the reason step's catalog, which lists what a plan
// may name. Everything else walks the objective's domains, and a discovered
// tool belongs to no domain an objective declares — an MCP server is a tool
// source, not a pack, and the twin that may reach it is decided by an adapter
// binding rather than by the objective's subject matter.
//
// So the catalog asks the environments that were actually built for this twin,
// which is also what confines the listing: a twin bound to one MCP instance is
// shown that instance's tools and not another tenant's.
type ToolSource interface {
	// ProvidedCapabilities names the capabilities this environment executes
	// for the twin it was built for, in a stable order.
	ProvidedCapabilities() []capability.CapabilityID
}
