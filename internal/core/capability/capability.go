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
