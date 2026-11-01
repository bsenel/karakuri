package objective

import (
	"math"
	"testing"
)

// Phase 41 step 7 part C: an at_start baseline freezes the one value the
// samples reduce to. all and k_of_n have no statistic of their own, so their
// baseline is the median, and a NaN is never part of it.
func TestReduceIsTheValueAnAtStartBaselineFreezes(t *testing.T) {
	samples := []float64{120, math.NaN(), 100, 300, 110}
	for _, tc := range []struct {
		reducer Reducer
		want    float64
		ok      bool
	}{
		{"", 115, true},
		{ReducerMedian, 115, true},
		{ReducerAll, 115, true},
		{ReducerKOfN, 115, true},
		{ReducerMin, 100, true},
		{ReducerMax, 300, true},
		{ReducerMean, 157.5, true},
		{ReducerP95, 300, true},
		{"any", 0, false},
	} {
		got, ok := Criterion{Reducer: tc.reducer}.Reduce(samples)
		if ok != tc.ok || got != tc.want {
			t.Errorf("reducer %q: Reduce = %v, %v; want %v, %v", tc.reducer, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := (Criterion{}).Reduce([]float64{math.NaN()}); ok {
		t.Error("Reduce of only NaNs reported a value; want none")
	}
}
