package loop

import (
	"encoding/json"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// riskPack is a pack whose only contribution is the templates it declares,
// each with the risk class its author gave it.
type riskPack struct {
	manyAgentPack
	templates []objective.Template
}

func (p *riskPack) ObjectiveTemplates() []objective.Template { return p.templates }
func (p *riskPack) AgentDefinitions() []coreagent.Definition { return nil }

// riskTemplates are the templates the registry knows in these tests: one of
// each declared class, and one whose author said nothing.
func riskTemplates() []objective.Template {
	return []objective.Template{
		{ID: "tmpl-green-build", Title: "keep the build green", Domain: "test", Risk: objective.RiskConsequential},
		{ID: "tmpl-tidy", Title: "tidy the changelog", Domain: "test", Risk: objective.RiskRoutine},
		{ID: "tmpl-triage", Title: "triage a patient message", Domain: "test", Risk: objective.RiskHigh},
		{ID: "tmpl-unsaid", Title: "nobody classified this", Domain: "test"},
	}
}

// payloadRiskClass reads risk_class out of a row's payload, failing when the
// key is absent: an absent key and "unclassified" are different records.
func payloadRiskClass(t *testing.T, row storage.ToolEvent) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v: %s", err, row.PayloadJSON)
	}
	got, ok := payload["risk_class"]
	if !ok {
		t.Fatalf("payload has no risk_class: %s", row.PayloadJSON)
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("payload risk_class = %v (%T), want a string", got, got)
	}
	return s
}

// A decision row names the template the objective was built from, but a
// template's classification can be edited after the fact. The row therefore
// carries the class the author had declared when the decision was taken, on
// the escalation and on the execute row alike, in the payload beside the
// bounds: how the work was regarded is part of what produced the decision.
func TestDecisionRowsRecordTheTemplateRiskClass(t *testing.T) {
	templates := map[string]struct {
		templateID string
		want       string
	}{
		"consequential template": {templateID: "tmpl-green-build", want: "consequential"},
		"routine template":       {templateID: "tmpl-tidy", want: "routine"},
		"high template":          {templateID: "tmpl-triage", want: "high"},
		// The author declared the template and said nothing about its risk.
		"template with no class": {templateID: "tmpl-unsaid", want: "unclassified"},
		// An objective written by hand came from no template.
		"no template": {templateID: "", want: "unclassified"},
		// The template was removed, or the objective outlived its pack.
		"unknown template": {templateID: "tmpl-gone", want: "unclassified"},
	}
	for name, tmpl := range templates {
		for kindName, tc := range decisionCases {
			t.Run(name+"/"+kindName, func(t *testing.T) {
				obj := objective.Objective{ID: "obj-risk", Title: "keep the build green", Domain: "test", TemplateID: tmpl.templateID}
				svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
				svc.domReg = registryWith(t, &riskPack{manyAgentPack: manyAgentPack{id: "test"}, templates: riskTemplates()})
				def := provenanceAgent("anthropic", tc.bounds)

				runSync(svc, "loop-risk-0001", loop.Request{
					Objective: obj, Agent: def, MaxIter: 1, AutonomyRung: objective.AutonomyPropose,
				}, tc.pending)

				rows := rec.ofKind(tc.kind)
				if len(rows) != 1 {
					t.Fatalf("%s rows = %d, want 1", tc.kind, len(rows))
				}
				if got := payloadRiskClass(t, rows[0]); got != tmpl.want {
					t.Errorf("payload risk_class = %q, want %q", got, tmpl.want)
				}
				// The class is recorded beside the template, not instead of it.
				if rows[0].TemplateID != tmpl.templateID {
					t.Errorf("template id = %q, want %q", rows[0].TemplateID, tmpl.templateID)
				}
			})
		}
	}
}

// A template is found by its ID wherever it is declared: a cross-domain
// objective's template may live in a pack other than its primary domain's.
func TestRiskClassIsFoundInWhicheverPackDeclaresTheTemplate(t *testing.T) {
	for kindName, tc := range decisionCases {
		t.Run(kindName, func(t *testing.T) {
			obj := objective.Objective{ID: "obj-risk-x", Title: "keep the build green", Domain: "test", TemplateID: "tmpl-triage"}
			svc, rec := provenanceFixture(t, &provenanceProvider{name: "anthropic", model: "model-a"}, obj)
			svc.domReg = registryWith(t, &riskPack{manyAgentPack: manyAgentPack{id: "elsewhere"}, templates: riskTemplates()})
			def := provenanceAgent("anthropic", tc.bounds)

			runSync(svc, "loop-risk-0002", loop.Request{Objective: obj, Agent: def, MaxIter: 1}, tc.pending)

			rows := rec.ofKind(tc.kind)
			if len(rows) != 1 {
				t.Fatalf("%s rows = %d, want 1", tc.kind, len(rows))
			}
			if got := payloadRiskClass(t, rows[0]); got != "high" {
				t.Errorf("payload risk_class = %q, want %q", got, "high")
			}
		})
	}
}
