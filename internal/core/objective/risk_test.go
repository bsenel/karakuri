package objective

import (
	"encoding/json"
	"testing"
)

func TestRiskClass_ValidAcceptsExactlyTheSet(t *testing.T) {
	for _, r := range []RiskClass{RiskUnclassified, RiskRoutine, RiskConsequential, RiskHigh} {
		if !r.Valid() {
			t.Errorf("%q is in the set and Valid() rejected it", string(r))
		}
	}
	for _, r := range []RiskClass{"critical", "HIGH", "unclassified ", "unclassified", " routine", "Routine", "low"} {
		if r.Valid() {
			t.Errorf("%q is outside the set and Valid() accepted it", string(r))
		}
	}
}

func TestRiskClass_ZeroValueIsUnclassified(t *testing.T) {
	var r RiskClass
	if r != RiskUnclassified {
		t.Fatalf("zero value is %q, want RiskUnclassified", string(r))
	}
	if got := r.String(); got != "unclassified" {
		t.Errorf("zero value String() = %q, want %q", got, "unclassified")
	}
}

func TestRiskClass_StringReturnsTheValue(t *testing.T) {
	for r, want := range map[RiskClass]string{
		RiskRoutine:       "routine",
		RiskConsequential: "consequential",
		RiskHigh:          "high",
	} {
		if got := r.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestTemplate_MarshalsRiskUnderRiskKey(t *testing.T) {
	raw, err := json.Marshal(Template{ID: "software.objective.fixture", Domain: "software", Risk: RiskConsequential})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["risk"] != "consequential" {
		t.Errorf(`"risk" = %v, want "consequential" in %s`, got["risk"], raw)
	}

	// An unclassified template still carries the key: a reader must be able to
	// tell "not classified" from "this server does not report risk".
	raw, err = json.Marshal(Template{ID: "software.objective.fixture", Domain: "software"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["risk"]; !ok || v != "" {
		t.Errorf(`unclassified template: "risk" = %v (present=%v), want "" in %s`, v, ok, raw)
	}
}
