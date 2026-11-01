package objective

import (
	"encoding/json"
	"math"
	"testing"
)

// measured builds a measured criterion, metric op target unit
// (docs/roadmap.md, Phase 41 decision 3, 'Form').
func measured(op Op, target float64, unit string) Criterion {
	return Criterion{
		ID:       "c",
		Kind:     KindMeasured,
		Verifier: "pack.verify.share",
		Metric:   "share",
		Op:       op,
		Target:   &target,
		Unit:     unit,
	}
}

// A measured criterion travels with the field names the roadmap gives it:
// kind, and the measured form's metric, op, target and unit (Phase 41
// decision 5, the --criterion flag 'id=..;kind=measured;metric=..;op=..;target=..;unit=..').
func TestMeasuredCriterionRoundTripsThroughJSON(t *testing.T) {
	in := measured(OpGreaterEqual, 0.8, "ratio")

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]any{
		"kind": "measured", "metric": "share", "op": ">=", "target": 0.8, "unit": "ratio",
	} {
		if wire[field] != want {
			t.Errorf("%s = %v on the wire, want %v (%s)", field, wire[field], want, data)
		}
	}

	var out Criterion
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != KindMeasured || out.Metric != "share" || out.Op != OpGreaterEqual ||
		out.Target == nil || *out.Target != 0.8 || out.Unit != "ratio" {
		t.Errorf("read back %+v", out)
	}
}

// The three kinds are spelt as the roadmap spells them (Phase 41 decision 1).
func TestCriterionKindsAreJudgedVerifiedAndMeasured(t *testing.T) {
	for kind, want := range map[CriterionKind]string{
		KindJudged: "judged", KindVerified: "verified", KindMeasured: "measured",
	} {
		if string(kind) != want {
			t.Errorf("kind %q, want %q", kind, want)
		}
	}
}

// The comparison is plain arithmetic: 0.81 against >= 0.8 is met and 0.79 is
// not, and each operator is checked on both sides of its boundary.
func TestMeasuredComparisonSettlesAgainstTheTarget(t *testing.T) {
	cases := []struct {
		name  string
		op    Op
		value float64
		want  CriterionState
	}{
		{"0.81 >= 0.8", OpGreaterEqual, 0.81, CriterionMet},
		{"0.79 >= 0.8", OpGreaterEqual, 0.79, CriterionNotMet},
		{"0.8 >= 0.8", OpGreaterEqual, 0.8, CriterionMet},
		{"0.8 > 0.8", OpGreater, 0.8, CriterionNotMet},
		{"0.81 > 0.8", OpGreater, 0.81, CriterionMet},
		{"0.8 <= 0.8", OpLessEqual, 0.8, CriterionMet},
		{"0.81 <= 0.8", OpLessEqual, 0.81, CriterionNotMet},
		{"0.8 < 0.8", OpLess, 0.8, CriterionNotMet},
		{"0.79 < 0.8", OpLess, 0.79, CriterionMet},
		{"0.8 == 0.8", OpEqual, 0.8, CriterionMet},
		{"0.81 == 0.8", OpEqual, 0.81, CriterionNotMet},
		{"0.79 == 0.8", OpEqual, 0.79, CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, reason := measured(tc.op, 0.8, "ratio").Compare(tc.value, "ratio")
			if state != tc.want {
				t.Errorf("state = %q, want %q", state, tc.want)
			}
			if reason != "" {
				t.Errorf("a settled comparison carries reason %q", reason)
			}
		})
	}
}

// The same comparison gives the same answer every time; nothing in it reads a
// clock, a model or a previous call.
func TestMeasuredComparisonIsDeterministic(t *testing.T) {
	c := measured(OpGreaterEqual, 0.8, "ratio")
	first, _ := c.Compare(0.81, "ratio")
	for range 100 {
		if got, _ := c.Compare(0.81, "ratio"); got != first {
			t.Fatalf("Compare gave %q after %q", got, first)
		}
	}
	if first != CriterionMet {
		t.Errorf("0.81 >= 0.8 is %q, want met", first)
	}
}

// A value in another unit is never met by accident, whichever side of the
// target its number falls. The unit must equal the declared unit
// (docs/roadmap.md:5557-5558), and an inconclusive state always carries a
// reason (docs/roadmap.md:5470). Which reason is not given by the roadmap, so
// it is not pinned here; not_met is ruled out because a measurement in the
// wrong unit says nothing about the work (docs/roadmap.md:5473-5474).
func TestMeasuredComparisonInAnotherUnitIsNeverMet(t *testing.T) {
	for _, value := range []float64{0.5, 81, 1e9} {
		state, reason := measured(OpGreaterEqual, 0.8, "ratio").Compare(value, "percent")
		if state != CriterionInconclusive {
			t.Errorf("%v percent against >= 0.8 ratio: state = %q, want inconclusive", value, state)
		}
		if reason == "" {
			t.Errorf("%v percent against >= 0.8 ratio: inconclusive carries no reason", value)
		}
	}
}

// NaN is no data, never a silent zero and never a pass, for every operator
// (docs/roadmap.md:5473-5474).
func TestMeasuredComparisonOfNaNIsNoData(t *testing.T) {
	for _, op := range []Op{OpLess, OpLessEqual, OpGreater, OpGreaterEqual, OpEqual} {
		state, reason := measured(op, 0.8, "ratio").Compare(math.NaN(), "ratio")
		if state != CriterionInconclusive || reason != ReasonNoData {
			t.Errorf("NaN %s 0.8: %q, %q; want inconclusive: no_data", op, state, reason)
		}
	}
}
