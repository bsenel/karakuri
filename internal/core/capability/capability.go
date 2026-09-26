package capability

type CapabilityID string

type Capability struct {
	ID           CapabilityID `json:"id"`
	Name         string       `json:"name"`
	Domain       string       `json:"domain"`
	Description  string       `json:"description,omitempty"`
	InputSchema  Schema       `json:"input_schema,omitempty"`
	OutputSchema Schema       `json:"output_schema,omitempty"`
	Verifiable   bool         `json:"verifiable,omitempty"`
	LLMHints     LLMHints     `json:"llm_hints,omitempty"`

	// NeedsWorkspace says this capability writes files and must be given an
	// isolated git worktree before it runs. The loop provisions one and puts
	// its path and branch in the action's params.
	//
	// Declared rather than inferred. The loop used to decide by matching the
	// capability's name against ".write_code" and ".write_test", which meant
	// software.act.delegate_to_cli — the one capability with a working
	// implementation — never got a workspace, while the two that did get one
	// had no implementation at all. A capability that needs a workspace is
	// something only the capability knows.
	//
	// Read it through GrantsWorkspace rather than directly: a capability
	// discovered from an MCP server never gets one, whatever it declares.
	NeedsWorkspace bool `json:"needs_workspace,omitempty"`
}

// GrantsWorkspace reports whether the loop must provision a git worktree before
// this capability runs.
//
// It is NeedsWorkspace for everything a pack declared, and false for everything
// in the reserved MCP namespace — the first of the four bounds in ADR 022, at
// the one place that would otherwise trust the declaration. A discovered tool's
// description is written by a server this repository did not write, and "this
// tool edits files" arriving over a socket is not a reason to cut a branch.
//
// Asked of the ID rather than guaranteed by NewMCPCapability alone, because the
// registry entry is a plain struct: anything that builds or rewrites one could
// set the field, and the check has to hold for the capability the loop actually
// looks up.
func (c Capability) GrantsWorkspace() bool {
	if IsMCPCapability(c.ID) {
		return false
	}
	return c.NeedsWorkspace
}

type Schema struct {
	Type       string                    `json:"type,omitempty"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

type SchemaProperty struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type LLMHints struct {
	PreferredProvider string  `json:"preferred_provider,omitempty"`
	FallbackProvider  string  `json:"fallback_provider,omitempty"`
	TemperatureMin    float64 `json:"temperature_min,omitempty"`
	TemperatureMax    float64 `json:"temperature_max,omitempty"`
}
