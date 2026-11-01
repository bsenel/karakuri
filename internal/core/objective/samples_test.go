package objective

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// Phase 41 step 7 part A: a measured criterion takes samples, and a reducer
// turns them into one state (docs/roadmap.md, Phase 41 decision 3, 'Repeated
// runs').

// sampled builds a measured criterion over n samples with the given reducer.
func sampled(reducer Reducer, n int, op Op, target float64) Criterion {
	c := measured(op, target, "ms")
	c.Samples = n
	c.Reducer = reducer
	return c
}

// p95Values is nineteen 100s and one 1000. Its p95 is 100 by nearest rank and
// 145 by linear interpolation, so either reading is under 200, while its max
// is 1000.
func p95Values() []float64 {
	v := make([]float64, 0, 20)
	for range 19 {
		v = append(v, 100)
	}
	return append(v, 1000)
}

// Each reducer settles the samples by its own statistic. Every reducer has a
// row where it is met and a row with the same samples where a wrong reducer is
// not, so a reducer that is ignored or swapped fails a row.
func TestCompareSamplesSettlesThroughTheReducer(t *testing.T) {
	cases := []struct {
		name    string
		reducer Reducer
		values  []float64
		op      Op
		target  float64
		want    CriterionState
	}{
		// The roadmap's acceptance: median 12, while the mean would be 24.
		{"median of [10 50 12] <= 15", ReducerMedian, []float64{10, 50, 12}, OpLessEqual, 15, CriterionMet},
		{"mean of [10 50 12] <= 15", ReducerMean, []float64{10, 50, 12}, OpLessEqual, 15, CriterionNotMet},

		{"min of [5 20 30] <= 10", ReducerMin, []float64{5, 20, 30}, OpLessEqual, 10, CriterionMet},
		{"max of [5 20 30] <= 10", ReducerMax, []float64{5, 20, 30}, OpLessEqual, 10, CriterionNotMet},

		{"max of [70 80 95] >= 90", ReducerMax, []float64{70, 80, 95}, OpGreaterEqual, 90, CriterionMet},
		{"min of [70 80 95] >= 90", ReducerMin, []float64{70, 80, 95}, OpGreaterEqual, 90, CriterionNotMet},

		{"mean of [10 10 40] <= 20", ReducerMean, []float64{10, 10, 40}, OpLessEqual, 20, CriterionMet},
		{"max of [10 10 40] <= 20", ReducerMax, []float64{10, 10, 40}, OpLessEqual, 20, CriterionNotMet},

		{"p95 of nineteen 100s and a 1000 <= 200", ReducerP95, p95Values(), OpLessEqual, 200, CriterionMet},
		{"max of nineteen 100s and a 1000 <= 200", ReducerMax, p95Values(), OpLessEqual, 200, CriterionNotMet},
		// p95 is not the median: nine 10s and a 500 have a median of 10 and a
		// p95 above 100 by nearest rank (500) and by interpolation (279.5).
		{"p95 of nine 10s and a 500 <= 100", ReducerP95, []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 500}, OpLessEqual, 100, CriterionNotMet},
		{"median of nine 10s and a 500 <= 100", ReducerMedian, []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 500}, OpLessEqual, 100, CriterionMet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := sampled(tc.reducer, len(tc.values), tc.op, tc.target)
			state, reason := c.CompareSamples(tc.values, "ms")
			if state != tc.want || reason != "" {
				t.Errorf("CompareSamples(%v) = %q, %q; want %q with no reason", tc.values, state, reason, tc.want)
			}
		})
	}
}

// all is met only when every sample meets the target, and k_of_n when at
// least k of them do. A pass/fail signal is 1 or 0 against >= 1.
func TestCompareSamplesCountsPassesForAllAndKOfN(t *testing.T) {
	cases := []struct {
		name    string
		reducer Reducer
		k       int
		values  []float64
		want    CriterionState
	}{
		{"all, 3 of 3", ReducerAll, 0, []float64{1, 1, 1}, CriterionMet},
		{"all, 2 of 3", ReducerAll, 0, []float64{1, 0, 1}, CriterionNotMet},
		{"all, 0 of 3", ReducerAll, 0, []float64{0, 0, 0}, CriterionNotMet},
		{"k_of_n k=2, 2 of 3", ReducerKOfN, 2, []float64{1, 0, 1}, CriterionMet},
		{"k_of_n k=2, 3 of 3", ReducerKOfN, 2, []float64{1, 1, 1}, CriterionMet},
		{"k_of_n k=2, 1 of 3", ReducerKOfN, 2, []float64{0, 0, 1}, CriterionNotMet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := sampled(tc.reducer, len(tc.values), OpGreaterEqual, 1)
			c.K = tc.k
			state, reason := c.CompareSamples(tc.values, "ms")
			if state != tc.want || reason != "" {
				t.Errorf("CompareSamples(%v) = %q, %q; want %q with no reason", tc.values, state, reason, tc.want)
			}
		})
	}
}

// Below min_samples nothing is settled, however good the samples are: the
// roadmap's acceptance, two samples with min_samples: 3 (docs/roadmap.md,
// Phase 41 step 7).
func TestCompareSamplesBelowMinSamplesIsTooFewSamples(t *testing.T) {
	for _, reducer := range []Reducer{ReducerMedian, ReducerAll, ReducerMean} {
		t.Run(string(reducer), func(t *testing.T) {
			c := sampled(reducer, 3, OpLessEqual, 15)
			c.MinSamples = 3
			state, reason := c.CompareSamples([]float64{10, 12}, "ms")
			if state != CriterionInconclusive || reason != ReasonTooFewSamples {
				t.Errorf("two samples with min_samples 3 = %q, %q; want inconclusive: too_few_samples", state, reason)
			}
		})
	}

	t.Run("at min_samples it settles", func(t *testing.T) {
		c := sampled(ReducerMedian, 3, OpLessEqual, 15)
		c.MinSamples = 3
		state, reason := c.CompareSamples([]float64{10, 50, 12}, "ms")
		if state != CriterionMet || reason != "" {
			t.Errorf("three samples with min_samples 3 = %q, %q; want met", state, reason)
		}
	})
}

// A measured criterion with several samples and no reducer is a noisy signal,
// and defaults to the median (docs/roadmap.md, Phase 41 decision 3). [10 50 12]
// against <= 15 is met only by the median: all, max and mean all say not_met.
func TestCompareSamplesDefaultsToTheMedian(t *testing.T) {
	c := sampled("", 3, OpLessEqual, 15)
	state, reason := c.CompareSamples([]float64{10, 50, 12}, "ms")
	if state != CriterionMet || reason != "" {
		t.Errorf("no reducer over [10 50 12] <= 15 = %q, %q; want met by the median", state, reason)
	}
}

// A NaN is no data, so it is dropped before the samples are counted: it is not
// a failing sample under all, and it does not count towards min_samples.
func TestCompareSamplesDropsNaNBeforeCounting(t *testing.T) {
	nan := math.NaN()

	t.Run("not counted towards min_samples", func(t *testing.T) {
		c := sampled(ReducerMedian, 3, OpLessEqual, 15)
		c.MinSamples = 3
		state, reason := c.CompareSamples([]float64{10, nan, 12}, "ms")
		if state != CriterionInconclusive || reason != ReasonTooFewSamples {
			t.Errorf("[10 NaN 12] with min_samples 3 = %q, %q; want inconclusive: too_few_samples", state, reason)
		}
	})

	t.Run("not a failing sample under all", func(t *testing.T) {
		c := sampled(ReducerAll, 3, OpGreaterEqual, 1)
		state, reason := c.CompareSamples([]float64{1, nan, 1}, "ms")
		if state != CriterionMet || reason != "" {
			t.Errorf("all over [1 NaN 1] >= 1 = %q, %q; want met", state, reason)
		}
	})

	t.Run("not a value of the median", func(t *testing.T) {
		// Without the NaN the median of [10 12 50] is 12; a NaN sorted in
		// anywhere would move it.
		c := sampled(ReducerMedian, 4, OpLessEqual, 15)
		state, reason := c.CompareSamples([]float64{nan, 10, 50, 12}, "ms")
		if state != CriterionMet || reason != "" {
			t.Errorf("median over [NaN 10 50 12] <= 15 = %q, %q; want met", state, reason)
		}
	})
}

// No samples at all is no data, never a pass and never not_met.
func TestCompareSamplesWithNoSamplesIsNoData(t *testing.T) {
	for _, reducer := range []Reducer{ReducerMedian, ReducerAll, ReducerKOfN, ReducerP95, ReducerMin, ReducerMax, ReducerMean} {
		t.Run(fmt.Sprint(reducer), func(t *testing.T) {
			c := sampled(reducer, 3, OpLessEqual, 15)
			c.K = 1
			for _, values := range [][]float64{nil, {}} {
				state, reason := c.CompareSamples(values, "ms")
				if state != CriterionInconclusive || reason != ReasonNoData {
					t.Errorf("CompareSamples(%v) = %q, %q; want inconclusive: no_data", values, state, reason)
				}
			}
		})
	}
}

// Samples in another unit than the criterion's are what Compare calls a
// verifier emitting something other than it declared.
func TestCompareSamplesInAnotherUnitIsAVerifierError(t *testing.T) {
	c := sampled(ReducerMedian, 3, OpLessEqual, 15)
	state, reason := c.CompareSamples([]float64{10, 50, 12}, "s")
	if state != CriterionInconclusive || reason != ReasonVerifierError {
		t.Errorf("samples in s for a criterion in ms = %q, %q; want inconclusive: verifier_error", state, reason)
	}
}

// fieldsOf is the criterion as the wire carries it.
func fieldsOf(t *testing.T, c Criterion) map[string]any {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}

// The new fields travel under the names the roadmap gives them, and none of
// them is written when it is unset.
func TestSampledCriterionFieldNames(t *testing.T) {
	c := sampled(ReducerKOfN, 3, OpGreaterEqual, 1)
	c.K = 2
	c.MinSamples = 3
	got := fieldsOf(t, c)
	for field, want := range map[string]any{"samples": 3.0, "reducer": "k_of_n", "k": 2.0, "min_samples": 3.0} {
		if got[field] != want {
			t.Errorf("%s = %v, want %v", field, got[field], want)
		}
	}

	plain := fieldsOf(t, measured(OpGreaterEqual, 0.8, "ratio"))
	for _, field := range []string{"samples", "reducer", "k", "min_samples"} {
		if _, ok := plain[field]; ok {
			t.Errorf("%s written on a criterion that does not set it", field)
		}
	}
}
