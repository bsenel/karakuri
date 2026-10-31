package capability

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Phase 41 step 4: a verifier declares each metric it can emit, with its
// name, unit, direction and typed parameters (docs/roadmap.md, Phase 41
// decision 2, 'Declaration'). Phase 39's pack manifest is not on main, so the
// declaration is read off the capability itself.

func coverageVerifier() Capability {
	return Capability{
		ID:         "software.verify.coverage",
		Domain:     "software",
		Verifiable: true,
		Metrics: []Metric{
			{
				Name:      "coverage",
				Unit:      "ratio",
				Direction: DirectionHigherIsBetter,
				Params:    []MetricParam{{Name: "packages", Type: ParamString}},
			},
			{Name: "duration", Unit: "s", Direction: DirectionLowerIsBetter},
		},
	}
}

// The loop reads a criterion's metric off the capability the registry holds
// for its verifier, so the declaration has to survive registration and be
// found by name; a name the verifier does not declare is not found.
func TestARegisteredVerifierDeclaresItsMetricsByName(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(coverageVerifier()); err != nil {
		t.Fatal(err)
	}
	c, ok := reg.Get("software.verify.coverage")
	if !ok {
		t.Fatal("the verifier is not registered")
	}

	got, ok := c.DeclaredMetric("coverage")
	if !ok {
		t.Fatal("coverage is declared and was not found")
	}
	want := coverageVerifier().Metrics[0]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("declared = %+v, want %+v", got, want)
	}

	if m, ok := c.DeclaredMetric("p95_latency_ms"); ok {
		t.Errorf("an undeclared metric was found: %+v", m)
	}
}

// The declaration sits beside output_schema rather than inside it, so a reader
// finds a metric's unit without interpreting a JSON Schema.
func TestTheMetricDeclarationSitsBesideTheOutputSchema(t *testing.T) {
	c := coverageVerifier()
	c.OutputSchema = Schema{Type: "object"}

	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["metrics"]; !ok {
		t.Fatalf("no top-level metrics in %s", raw)
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(top["output_schema"], &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["metrics"]; ok {
		t.Errorf("metrics leaked into the output schema: %s", top["output_schema"])
	}

	var back Capability
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Metrics, c.Metrics) {
		t.Errorf("metrics = %+v after a round trip, want %+v", back.Metrics, c.Metrics)
	}
}
