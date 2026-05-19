package capability

// Universal capabilities available to all agents regardless of domain.
var Universal = []Capability{
	{
		ID:          "observe.fetch_signal",
		Name:        "Fetch Signal",
		Domain:      "universal",
		Description: "Fetch a generic signal from an environment",
		InputSchema: Schema{
			Type:     "object",
			Required: []string{"env_id"},
			Properties: map[string]SchemaProperty{
				"env_id": {Type: "string", Description: "Environment ID to query"},
				"query":  {Type: "string", Description: "Optional query filter"},
			},
		},
		OutputSchema: Schema{Type: "object"},
	},
	{
		ID:          "reason.synthesize",
		Name:        "Synthesize",
		Domain:      "universal",
		Description: "Synthesize information from multiple sources into a coherent summary",
		InputSchema: Schema{
			Type:     "object",
			Required: []string{"inputs"},
			Properties: map[string]SchemaProperty{
				"inputs": {Type: "string", Description: "Inputs to synthesize"},
				"format": {Type: "string", Description: "Output format"},
			},
		},
		OutputSchema: Schema{Type: "object"},
	},
	{
		ID:          "reason.plan",
		Name:        "Plan",
		Domain:      "universal",
		Description: "Produce a structured action plan toward an objective",
		InputSchema: Schema{
			Type:     "object",
			Required: []string{"objective"},
			Properties: map[string]SchemaProperty{
				"objective": {Type: "string", Description: "Objective description"},
				"context":   {Type: "string", Description: "Additional context"},
			},
		},
		OutputSchema: Schema{Type: "object"},
	},
	{
		ID:          "reason.evaluate",
		Name:        "Evaluate",
		Domain:      "universal",
		Description: "Evaluate an artifact or plan against defined criteria",
		InputSchema: Schema{
			Type:     "object",
			Required: []string{"artifact", "criteria"},
			Properties: map[string]SchemaProperty{
				"artifact": {Type: "string", Description: "Content to evaluate"},
				"criteria": {Type: "string", Description: "Evaluation criteria"},
			},
		},
		OutputSchema: Schema{Type: "object"},
		Verifiable:   true,
	},
}
