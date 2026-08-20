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
	NeedsWorkspace bool `json:"needs_workspace,omitempty"`
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
