package capability

type CapabilityID string

type Capability struct {
	ID           CapabilityID `json:"id"`
	Name         string       `json:"name"`
	Domain       string       `json:"domain"`
	Description  string       `json:"description,omitempty"`
	InputSchema  Schema       `json:"input_schema,omitempty"`
	OutputSchema Schema       `json:"output_schema,omitempty"`
	// Metrics declares each named number this capability can emit as a
	// verifier (Phase 41, decision 2, 'Declaration'). It sits beside
	// OutputSchema rather than inside a JSON Schema, so validation and the
	// loop read it without interpreting one.
	//
	// The roadmap puts the declaration in the pack manifest (Phase 39, ADR
	// 028), and that manifest is not on main, so it lives on the capability
	// a pack registers until it is.
	Metrics    []Metric `json:"metrics,omitempty"`
	Verifiable bool     `json:"verifiable,omitempty"`
	LLMHints   LLMHints `json:"llm_hints,omitempty"`

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

// Metric is one number a verifier declares it can emit: its name, the unit
// every value of it is in, which way is better, and the parameters it takes.
// Which names exist is a pack's business; the core knows only the shape.
type Metric struct {
	Name      string        `json:"name"`
	Unit      string        `json:"unit"`
	Direction Direction     `json:"direction"`
	Params    []MetricParam `json:"params,omitempty"`
}

// Direction says which way a metric improves.
type Direction string

const (
	DirectionHigherIsBetter Direction = "higher_is_better"
	DirectionLowerIsBetter  Direction = "lower_is_better"
)

// MetricParam is one typed parameter a metric takes, such as the packages a
// coverage figure is computed over.
type MetricParam struct {
	Name string    `json:"name"`
	Type ParamType `json:"type"`
}

// ParamType is the type of a metric parameter's value.
type ParamType string

const (
	ParamString  ParamType = "string"
	ParamNumber  ParamType = "number"
	ParamInteger ParamType = "integer"
	ParamBoolean ParamType = "boolean"
)

// DeclaredMetric returns the metric this capability declares under name.
func (c Capability) DeclaredMetric(name string) (Metric, bool) {
	return Metric{}, false
}

type LLMHints struct {
	PreferredProvider string  `json:"preferred_provider,omitempty"`
	FallbackProvider  string  `json:"fallback_provider,omitempty"`
	TemperatureMin    float64 `json:"temperature_min,omitempty"`
	TemperatureMax    float64 `json:"temperature_max,omitempty"`
}
