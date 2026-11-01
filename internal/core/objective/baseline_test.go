package objective

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Phase 41 step 7 part B: a baseline fixed by hand, a change against it, and
// no_baseline (docs/roadmap.md, Phase 41 decision 3 'Baselines').

// relative is a measured criterion whose target is a change against a frozen
// baseline; a nil baseline is one never measured.
func relative(op Op, target float64, unit string, change Change, baseline *float64) Criterion {
	c := measured(op, target, unit)
	c.Change = change
	c.BaselineValue = baseline
	if baseline != nil {
		c.Baseline = &Baseline{Value: *baseline}
	} else {
		c.Baseline = &Baseline{AtStart: true}
	}
	return c
}

func frozen(v float64) *float64 { return &v }

// The value compared with op target is the change from the baseline: absolute
// is value minus baseline, percent is that divided by |baseline| times 100.
func TestCompareSetsTheValueAgainstItsBaseline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		criterion Criterion
		value     float64
		want      CriterionState
	}{
		// p95 not more than 10% above a baseline of 200 ms.
		{"percent, 7.5% above", relative(OpLessEqual, 10, "ms", ChangePercent, frozen(200)), 215, CriterionMet},
		{"percent, exactly 10% above", relative(OpLessEqual, 10, "ms", ChangePercent, frozen(200)), 220, CriterionMet},
		{"percent, 15% above", relative(OpLessEqual, 10, "ms", ChangePercent, frozen(200)), 230, CriterionNotMet},
		{"percent, below the baseline", relative(OpLessEqual, 10, "ms", ChangePercent, frozen(200)), 150, CriterionMet},
		// Coverage must not drop below a baseline of 0.8.
		{"absolute, up 0.01", relative(OpGreaterEqual, 0, "ratio", ChangeAbsolute, frozen(0.8)), 0.81, CriterionMet},
		{"absolute, unchanged", relative(OpGreaterEqual, 0, "ratio", ChangeAbsolute, frozen(0.8)), 0.8, CriterionMet},
		{"absolute, down 0.01", relative(OpGreaterEqual, 0, "ratio", ChangeAbsolute, frozen(0.8)), 0.79, CriterionNotMet},
		// A raw 0.81 >= 0 would be met either way; against 0.8 it is a drop of
		// 0.19 and fails a target of at most 0.05 below.
		{"absolute, a raw value that would pass", relative(OpGreaterEqual, -0.05, "ratio", ChangeAbsolute, frozen(0.8)), 0.61, CriterionNotMet},
		// A baseline of 0 is a baseline for an absolute change.
		{"absolute against 0", relative(OpLessEqual, 2, "count", ChangeAbsolute, frozen(0)), 2, CriterionMet},
		{"absolute against 0, above", relative(OpLessEqual, 2, "count", ChangeAbsolute, frozen(0)), 3, CriterionNotMet},
		// Percent divides by the baseline's magnitude, so a rise from -50 to
		// -40 is +20%, not -20%.
		{"percent against a negative baseline", relative(OpGreaterEqual, 15, "usd", ChangePercent, frozen(-50)), -40, CriterionMet},
		{"percent against a negative baseline, short", relative(OpGreaterEqual, 15, "usd", ChangePercent, frozen(-50)), -45, CriterionNotMet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, reason := tc.criterion.Compare(tc.value, tc.criterion.Unit)
			if state != tc.want || reason != "" {
				t.Errorf("Compare(%v) = %q/%q, want %q", tc.value, state, reason, tc.want)
			}
		})
	}
}

// A change with no frozen baseline, or a percent change against a baseline of
// 0, has nothing to compare with: inconclusive: no_baseline, never not_met,
// whatever the value.
func TestAChangeWithoutABaselineIsNoBaseline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		criterion Criterion
	}{
		{"at_start never measured, percent", relative(OpLessEqual, 10, "ms", ChangePercent, nil)},
		{"at_start never measured, absolute", relative(OpGreaterEqual, 0, "ratio", ChangeAbsolute, nil)},
		{"percent against 0", relative(OpLessEqual, 10, "ms", ChangePercent, frozen(0))},
	} {
		for _, value := range []float64{-1e9, -1, 0, 0.5, 215, 1e9} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, value), func(t *testing.T) {
				state, reason := tc.criterion.Compare(value, tc.criterion.Unit)
				if state != CriterionInconclusive || reason != ReasonNoBaseline {
					t.Errorf("Compare(%v) = %q/%q, want inconclusive/no_baseline", value, state, reason)
				}
			})
		}
	}
}

// Samples are set against the baseline too: each one under all and k_of_n,
// the statistic under the others.
func TestCompareSamplesSetsSamplesAgainstTheBaseline(t *testing.T) {
	median := relative(OpLessEqual, 10, "ms", ChangePercent, frozen(200))
	median.Samples, median.Reducer = 3, ReducerMedian
	// The median 215 is 7.5% above 200.
	if state, reason := median.CompareSamples([]float64{210, 215, 400}, "ms"); state != CriterionMet {
		t.Errorf("median of [210 215 400] against 200 +10%% = %q/%q, want met", state, reason)
	}
	// The median 230 is 15% above.
	if state, reason := median.CompareSamples([]float64{220, 230, 240}, "ms"); state != CriterionNotMet {
		t.Errorf("median of [220 230 240] against 200 +10%% = %q/%q, want not_met", state, reason)
	}

	all := relative(OpGreaterEqual, 0, "ratio", ChangeAbsolute, frozen(0.8))
	all.Samples, all.Reducer = 2, ReducerAll
	if state, reason := all.CompareSamples([]float64{0.81, 0.79}, "ratio"); state != CriterionNotMet {
		t.Errorf("all of [0.81 0.79] against 0.8 = %q/%q, want not_met", state, reason)
	}

	unfrozen := relative(OpLessEqual, 10, "ms", ChangePercent, nil)
	unfrozen.Samples, unfrozen.Reducer, unfrozen.MinSamples = 3, ReducerMedian, 3
	if state, reason := unfrozen.CompareSamples([]float64{250, 260, 270}, "ms"); state != CriterionInconclusive || reason != ReasonNoBaseline {
		t.Errorf("samples with no baseline = %q/%q, want inconclusive/no_baseline", state, reason)
	}
}

// A criterion without a baseline and a change compares the raw value, as it
// always did; a frozen baseline_value alone changes nothing either.
func TestACriterionWithoutAChangeComparesTheRawValue(t *testing.T) {
	plain := measured(OpLessEqual, 10, "ms")
	if state, _ := plain.Compare(215, "ms"); state != CriterionNotMet {
		t.Errorf("215 <= 10 with no baseline = %q, want not_met", state)
	}
	if state, _ := plain.Compare(9, "ms"); state != CriterionMet {
		t.Errorf("9 <= 10 with no baseline = %q, want met", state)
	}
	plain.BaselineValue = frozen(200)
	if state, _ := plain.Compare(9, "ms"); state != CriterionMet {
		t.Errorf("9 <= 10 with a baseline but no change = %q, want met", state)
	}
}

// On the wire a baseline is "at_start" or a number, and nothing else.
func TestBaselineDecodesAtStartOrANumber(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want Baseline
	}{
		{`"at_start"`, Baseline{AtStart: true}},
		{`0.8`, Baseline{Value: 0.8}},
		{`200`, Baseline{Value: 200}},
		{`0`, Baseline{Value: 0}},
		{`-3.5`, Baseline{Value: -3.5}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var c Criterion
			if err := json.Unmarshal([]byte(`{"id":"c","baseline":`+tc.raw+`}`), &c); err != nil {
				t.Fatalf("unmarshal baseline %s: %v", tc.raw, err)
			}
			if c.Baseline == nil || *c.Baseline != tc.want {
				t.Errorf("baseline %s decoded to %+v, want %+v", tc.raw, c.Baseline, tc.want)
			}
		})
	}
}

func TestBaselineRefusesAnythingElse(t *testing.T) {
	for _, raw := range []string{`"at_end"`, `"AT_START"`, `""`, `"0.8"`, `true`, `{}`, `[0.8]`, `{"value":0.8}`} {
		t.Run(raw, func(t *testing.T) {
			var c Criterion
			if err := json.Unmarshal([]byte(`{"id":"c","baseline":`+raw+`}`), &c); err == nil {
				t.Errorf("baseline %s decoded to %+v, want an error", raw, c.Baseline)
			}
		})
	}
}

// A criterion with no baseline has none after decoding, and writes none.
func TestNoBaselineIsAbsentOnTheWire(t *testing.T) {
	var c Criterion
	if err := json.Unmarshal([]byte(`{"id":"c"}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Baseline != nil {
		t.Errorf("no baseline decoded to %+v, want nil", c.Baseline)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	for _, key := range []string{"baseline", "change", "baseline_value"} {
		if _, ok := wire[key]; ok {
			t.Errorf("a criterion without a baseline writes %q: %s", key, b)
		}
	}
}

// A baseline encodes as it was given, and a criterion round-trips with its
// change and frozen value.
func TestBaselineEncodesAsItWasGiven(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseline Baseline
		want     any
	}{
		{"at_start", Baseline{AtStart: true}, "at_start"},
		{"number", Baseline{Value: 0.8}, 0.8},
		{"zero", Baseline{Value: 0}, 0.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := measured(OpGreaterEqual, 0, "ratio")
			c.Baseline, c.Change, c.BaselineValue = &tc.baseline, ChangeAbsolute, frozen(0.8)
			b, err := json.Marshal(c)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(b, &wire); err != nil {
				t.Fatalf("unmarshal wire: %v", err)
			}
			if wire["baseline"] != tc.want {
				t.Errorf("baseline on the wire = %#v, want %#v (%s)", wire["baseline"], tc.want, b)
			}
			if wire["change"] != "absolute" || wire["baseline_value"] != 0.8 {
				t.Errorf("change = %#v, baseline_value = %#v; want absolute and 0.8 (%s)", wire["change"], wire["baseline_value"], b)
			}

			var back Criterion
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if back.Baseline == nil || *back.Baseline != tc.baseline || back.Change != ChangeAbsolute ||
				back.BaselineValue == nil || *back.BaselineValue != 0.8 {
				t.Errorf("round trip = baseline %+v, change %q, baseline_value %v", back.Baseline, back.Change, back.BaselineValue)
			}
		})
	}
}
