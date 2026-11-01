package objective

import (
	"encoding/json"
	"testing"
)

// A criterion gates completion unless it says otherwise, so every criterion
// written before Required existed — every template, every stored objective —
// keeps gating exactly as it did.
func TestCriterionIsRequiredUnlessItSaysOtherwise(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name     string
		required *bool
		want     bool
	}{
		{"unset", nil, true},
		{"required: true", &yes, true},
		{"required: false", &no, false},
	} {
		if got := (Criterion{ID: "c", Required: tc.required}).IsRequired(); got != tc.want {
			t.Errorf("%s: IsRequired() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The same default when the criterion arrives as JSON, which is how a stored
// objective and a template file both reach the loop.
func TestCriterionWithoutRequiredInJSONIsRequired(t *testing.T) {
	var c Criterion
	if err := json.Unmarshal([]byte(`{"id":"c1","description":"tests pass","weight":1}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !c.IsRequired() {
		t.Error("a criterion that said nothing about required was read as optional; it would stop gating completion")
	}

	if err := json.Unmarshal([]byte(`{"id":"c2","required":false}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.IsRequired() {
		t.Error(`"required": false was read as required`)
	}
}

// Met stays in JSON for the API's readers, derived from the state rather than
// stored beside it, so the two can never disagree on the wire.
func TestMetIsDerivedFromTheStateInJSON(t *testing.T) {
	for _, tc := range []struct {
		state   CriterionState
		reason  InconclusiveReason
		wantMet bool
	}{
		{CriterionMet, "", true},
		{CriterionNotMet, "", false},
		{CriterionInconclusive, ReasonVerifierError, false},
	} {
		raw, err := json.Marshal(Criterion{ID: "c", State: tc.state, Reason: tc.reason})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["met"] != tc.wantMet {
			t.Errorf("state %q: met = %v, want %v (%s)", tc.state, got["met"], tc.wantMet, raw)
		}
		if got["state"] != string(tc.state) {
			t.Errorf("state %q: state on the wire = %v (%s)", tc.state, got["state"], raw)
		}
		if tc.reason != "" && got["reason"] != string(tc.reason) {
			t.Errorf("state %q: reason = %v, want %q (%s)", tc.state, got["reason"], tc.reason, raw)
		}
	}
}

// An objective stored before states existed carries only "met". It keeps what
// it had: a met criterion reads back met, and is still required.
func TestAStoredMetCriterionReadsBackMet(t *testing.T) {
	var c Criterion
	if err := json.Unmarshal([]byte(`{"id":"c1","weight":2,"met":true}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.State != CriterionMet {
		t.Errorf("state = %q, want %q — a criterion stored as met lost its score", c.State, CriterionMet)
	}
	if !c.IsRequired() {
		t.Error("a stored criterion became optional on reading")
	}
}

// Met is derived, never a second source: a state on the wire wins over a "met"
// beside it, so a stale or hand-edited flag cannot turn an inconclusive
// criterion into a met one.
func TestTheStateWinsOverAMetFlagBesideIt(t *testing.T) {
	var c Criterion
	raw := `{"id":"c1","met":true,"state":"inconclusive","reason":"verifier_error"}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.State != CriterionInconclusive || c.Reason != ReasonVerifierError {
		t.Errorf("state = %q reason = %q, want inconclusive/verifier_error", c.State, c.Reason)
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["met"] != false {
		t.Errorf("an inconclusive criterion went back out as met: %s", out)
	}
}
