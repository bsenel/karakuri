package karakuri

import "github.com/bsenel/karakuri/internal/core/capability"

// Capability IDs this pack registers. All three analyse or draft; none of them
// changes anything.
//
// That is the pack's whole safety story. Karakuri improving itself means
// reading its own telemetry, deciding what is worth doing, and writing a
// proposal — and then the *software* pack's capabilities, in a git worktree,
// through a pull request an operator reviews, do the changing. Two packs is
// not tidiness: a single pack that could both decide what Karakuri should
// become and carry it out would be one bug away from a system that edits its
// own bounds.
const (
	CapAnalyseUsage   = "karakuri.analyse_usage"
	CapProposeRoadmap = "karakuri.propose_roadmap_phase"
	CapDraftADR       = "karakuri.draft_adr"
)

func karakuriCapabilities() []capability.Capability {
	prop := func(typ, desc string) capability.SchemaProperty {
		return capability.SchemaProperty{Type: typ, Description: desc}
	}

	return []capability.Capability{
		{
			ID:          CapAnalyseUsage,
			Name:        "Analyse Usage",
			Domain:      "karakuri",
			Description: "Read this deployment's telemetry and name what is limiting it, ranked",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"window":  prop("string", "How far back to look, e.g. 168h. Defaults to a week"),
					"twin_id": prop("string", "Narrow to one twin. Empty reads the whole deployment"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"bottlenecks":   prop("array", "What is going wrong, ranked by how often"),
					"approval_rate": prop("number", "Share of resolved escalations approved; -1 when nothing was decided"),
					"sense_ratio":   prop("number", "Cheap passes per expensive one — how well the two-tier split is working"),
					"summary":       prop("string", "One paragraph on what the numbers say"),
				},
			},
			Verifiable: true,
		},
		{
			ID:          CapProposeRoadmap,
			Name:        "Propose Roadmap Phase",
			Domain:      "karakuri",
			Description: "Draft a roadmap phase in the repository's established style, from evidence rather than from taste",
			InputSchema: capability.Schema{
				Type:     "object",
				Required: []string{"problem"},
				Properties: map[string]capability.SchemaProperty{
					"problem":  prop("string", "The limitation this phase would remove, in one sentence"),
					"evidence": prop("string", "The telemetry that says it is real — a phase proposed without this is a preference"),
					"scope":    prop("string", "What is in, and explicitly what is out"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"title":      prop("string", "Phase title"),
					"goal":       prop("string", "One sentence stating the outcome"),
					"steps":      prop("array", "Numbered steps"),
					"acceptance": prop("string", "How anybody would know it worked"),
				},
			},
			Verifiable: true,
		},
		{
			ID:          CapDraftADR,
			Name:        "Draft ADR",
			Domain:      "karakuri",
			Description: "Draft an architecture decision record: the decision, its consequences, and what was rejected",
			InputSchema: capability.Schema{
				Type:     "object",
				Required: []string{"decision"},
				Properties: map[string]capability.SchemaProperty{
					"decision":     prop("string", "The decision, stated as a claim rather than a topic"),
					"context":      prop("string", "What made it necessary"),
					"alternatives": prop("string", "What else was considered, and why it lost"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"title":        prop("string", "ADR title, stating the decision"),
					"body":         prop("string", "Markdown in the repository's ADR template"),
					"consequences": prop("array", "What follows from it, including what gets worse"),
				},
			},
			Verifiable: true,
		},
	}
}
