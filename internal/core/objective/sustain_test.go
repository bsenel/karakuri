package objective

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// Phase 41 step 8 part A: sustain is a not_met budget over a rolling window,
// not a streak (docs/roadmap.md, Phase 41 section '4. Sustain for standing
// objectives' and step 8's acceptance).

const day = 24 * time.Hour

// sustainNow is the time every table below evaluates at.
var sustainNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

// acceptanceSustain is step 8's: window 28d, budget 0.1, min_passes 3.
func acceptanceSustain() Sustain { return Sustain{Window: "28d", Budget: 0.1, MinPasses: 3} }

// daysAgo is a pass settled the given number of days before sustainNow.
func daysAgo(days float64, state CriterionState) CriterionPass {
	p := CriterionPass{At: sustainNow.Add(-time.Duration(days * float64(day))), State: state}
	if state == CriterionInconclusive {
		p.Reason = ReasonVerifierError
	}
	return p
}

// dailyPasses is n passes one day apart, the latest a day ago, every one met
// except those whose index (0 the oldest) is in notMet.
func dailyPasses(n int, notMet ...int) []CriterionPass {
	failed := make(map[int]bool, len(notMet))
	for _, i := range notMet {
		failed[i] = true
	}
	passes := make([]CriterionPass, 0, n)
	for i := range n {
		state := CriterionMet
		if failed[i] {
			state = CriterionNotMet
		}
		passes = append(passes, daysAgo(float64(n-i), state))
	}
	return passes
}

func TestSustainEvaluate(t *testing.T) {
	cases := []struct {
		name    string
		sustain Sustain
		passes  []CriterionPass
		since   time.Time
		want    SustainResult
	}{
		{
			name:    "ten settled passes with one not_met are sustained",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(10, 4),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			name:    "a second not_met breaks it",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(10, 4, 7),
			want:    SustainResult{State: CriterionNotMet, Settled: 10, NotMet: 2, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			name:    "no not_met leaves the whole budget",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(10),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			// The inconclusive pass is left out of both counts and spends no
			// budget, so the result is the ten passes' alone; it is still the
			// latest pass, and shown as such.
			name:    "an inconclusive pass changes neither count nor budget",
			sustain: acceptanceSustain(),
			passes:  append(dailyPasses(10, 4), daysAgo(0.5, CriterionInconclusive)),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: CriterionInconclusive},
		},
		{
			// One not_met in nine settled passes is over the budget. Were the
			// two inconclusive passes counted as settled, 1 in 11 would fit
			// it; were they counted as not_met, NotMet would be 3.
			name:    "inconclusive passes neither dilute nor spend the budget",
			sustain: acceptanceSustain(),
			passes: append(dailyPasses(9, 4),
				daysAgo(0.5, CriterionInconclusive), daysAgo(0.25, CriterionInconclusive)),
			want: SustainResult{State: CriterionNotMet, Settled: 9, NotMet: 1, BudgetLeft: 0, Latest: CriterionInconclusive},
		},
		{
			name:    "two settled passes in the window are too few",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(2),
			want:    SustainResult{State: CriterionInconclusive, Reason: ReasonTooFewSamples, Settled: 2, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			// Three passes, two of them inconclusive: one settled pass is
			// below min_passes however many passes ran.
			name:    "inconclusive passes do not count towards min_passes",
			sustain: acceptanceSustain(),
			passes: []CriterionPass{
				daysAgo(3, CriterionMet), daysAgo(2, CriterionInconclusive), daysAgo(1, CriterionInconclusive),
			},
			want: SustainResult{State: CriterionInconclusive, Reason: ReasonTooFewSamples, Settled: 1, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionInconclusive},
		},
		{
			name:    "no passes at all are too few",
			sustain: acceptanceSustain(),
			passes:  nil,
			want:    SustainResult{State: CriterionInconclusive, Reason: ReasonTooFewSamples, Settled: 0, NotMet: 0, BudgetLeft: 0.1},
		},
		{
			// The two not_met passes are 30 and 29 days old: out of the 28d
			// window, so the ten passes inside it hold no failure.
			name:    "a pass older than the window drops out",
			sustain: acceptanceSustain(),
			passes:  append([]CriterionPass{daysAgo(30, CriterionNotMet), daysAgo(29, CriterionNotMet)}, dailyPasses(10)...),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			// The window is rolling, never calendar: 28d before Oct 31 12:00
			// is Oct 3 12:00, so a pass on Oct 3 at 13:00 is inside it and
			// one at 11:00 is not, whatever the month.
			name:    "the window rolls by the hour, not the calendar",
			sustain: acceptanceSustain(),
			passes: append([]CriterionPass{
				{At: time.Date(2026, 10, 3, 11, 0, 0, 0, time.UTC), State: CriterionNotMet},
				{At: time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC), State: CriterionNotMet},
			}, dailyPasses(9)...),
			want: SustainResult{State: CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			// A person changed the target five days ago, so the window starts
			// there: the two not_met passes before it no longer count.
			name:    "a since later than now-window excludes the passes before it",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(10, 2, 3),
			since:   sustainNow.Add(-5*day - time.Hour),
			want:    SustainResult{State: CriterionMet, Settled: 5, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			name:    "a restart leaves too few passes until enough settle again",
			sustain: acceptanceSustain(),
			passes:  dailyPasses(10),
			since:   sustainNow.Add(-2*day - time.Hour),
			want:    SustainResult{State: CriterionInconclusive, Reason: ReasonTooFewSamples, Settled: 2, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			// A since older than the window restarts nothing the window has
			// not already dropped.
			name:    "a since older than the window changes nothing",
			sustain: acceptanceSustain(),
			passes:  append([]CriterionPass{daysAgo(30, CriterionNotMet), daysAgo(29, CriterionNotMet)}, dailyPasses(10)...),
			since:   sustainNow.Add(-60 * day),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 0, BudgetLeft: 0.1, Latest: CriterionMet},
		},
		{
			// budget 0 reads "no pass in the window ended not_met".
			name:    "budget 0 is broken by one not_met",
			sustain: Sustain{Window: "28d", Budget: 0, MinPasses: 3},
			passes:  dailyPasses(10, 4),
			want:    SustainResult{State: CriterionNotMet, Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			name:    "budget 0 with no not_met is sustained",
			sustain: Sustain{Window: "28d", Budget: 0, MinPasses: 3},
			passes:  dailyPasses(10),
			want:    SustainResult{State: CriterionMet, Settled: 10, NotMet: 0, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			// A criterion that spent budget and recovered shows its latest
			// state beside what is left, so the recovery can be seen.
			name:    "a recovered criterion shows its latest pass met",
			sustain: Sustain{Window: "28d", Budget: 0.25, MinPasses: 3},
			passes:  dailyPasses(4, 2),
			want:    SustainResult{State: CriterionMet, Settled: 4, NotMet: 1, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			name:    "the latest pass is the latest by time, whatever the order given",
			sustain: acceptanceSustain(),
			passes:  []CriterionPass{daysAgo(1, CriterionNotMet), daysAgo(3, CriterionMet), daysAgo(2, CriterionMet)},
			want:    SustainResult{State: CriterionNotMet, Settled: 3, NotMet: 1, BudgetLeft: 0, Latest: CriterionNotMet},
		},
		{
			name:    "a Go duration window",
			sustain: Sustain{Window: "72h", Budget: 0.5, MinPasses: 2},
			passes:  []CriterionPass{daysAgo(4, CriterionNotMet), daysAgo(2, CriterionNotMet), daysAgo(1, CriterionMet)},
			want:    SustainResult{State: CriterionMet, Settled: 2, NotMet: 1, BudgetLeft: 0, Latest: CriterionMet},
		},
		{
			name:    "min_passes 0 settles from one pass",
			sustain: Sustain{Window: "28d", Budget: 0.1},
			passes:  []CriterionPass{daysAgo(1, CriterionNotMet)},
			want:    SustainResult{State: CriterionNotMet, Settled: 1, NotMet: 1, BudgetLeft: 0, Latest: CriterionNotMet},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.sustain.Evaluate(tc.passes, sustainNow, tc.since)
			if got.State != tc.want.State || got.Reason != tc.want.Reason {
				t.Errorf("state = %q (%q), want %q (%q)", got.State, got.Reason, tc.want.State, tc.want.Reason)
			}
			if got.Settled != tc.want.Settled || got.NotMet != tc.want.NotMet {
				t.Errorf("settled = %d, not_met = %d, want %d and %d", got.Settled, got.NotMet, tc.want.Settled, tc.want.NotMet)
			}
			if math.Abs(got.BudgetLeft-tc.want.BudgetLeft) > 1e-9 {
				t.Errorf("budget left = %v, want %v", got.BudgetLeft, tc.want.BudgetLeft)
			}
			if got.Latest != tc.want.Latest {
				t.Errorf("latest = %q, want %q", got.Latest, tc.want.Latest)
			}
		})
	}
}

// Evaluate reads the passes reconcile recorded and leaves them as given: finding
// the latest by time must not sort the caller's record in place.
func TestSustainEvaluateLeavesThePassesAsGiven(t *testing.T) {
	passes := []CriterionPass{daysAgo(1, CriterionMet), daysAgo(3, CriterionMet), daysAgo(2, CriterionMet)}
	before := make([]CriterionPass, len(passes))
	copy(before, passes)
	got := acceptanceSustain().Evaluate(passes, sustainNow, time.Time{})
	if got.State != CriterionMet {
		t.Fatalf("state = %q, want met", got.State)
	}
	for i := range passes {
		if passes[i] != before[i] {
			t.Errorf("pass %d was changed to %+v, want %+v left as given", i, passes[i], before[i])
		}
	}
}

func TestParseWindow(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"28d":   28 * day,
		"1d":    day,
		"72h":   72 * time.Hour,
		"90m":   90 * time.Minute,
		"1h30m": 90 * time.Minute,
	} {
		t.Run(in, func(t *testing.T) {
			got, err := ParseWindow(in)
			if err != nil {
				t.Fatalf("ParseWindow(%q): %v", in, err)
			}
			if got != want {
				t.Errorf("ParseWindow(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestParseWindowRefusesWhatItCannotRead(t *testing.T) {
	for _, in := range []string{"", "28", "d", "fortnight", "1.5d", "28 d", "4w", "1mo"} {
		t.Run(in, func(t *testing.T) {
			if got, err := ParseWindow(in); err == nil {
				t.Errorf("ParseWindow(%q) = %v, want an error", in, got)
			}
		})
	}
}

// A criterion carries its sustain on the wire, and one without it carries no
// sustain key at all.
func TestCriterionSustainJSON(t *testing.T) {
	c := Criterion{ID: "held", Description: "held", Sustain: &Sustain{Window: "28d", Budget: 0.1, MinPasses: 3}}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire struct {
		Sustain map[string]any `json:"sustain"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire.Sustain["window"] != "28d" || wire.Sustain["budget"] != 0.1 || wire.Sustain["min_passes"] != float64(3) {
		t.Errorf("sustain on the wire = %v, want window 28d, budget 0.1, min_passes 3", wire.Sustain)
	}

	var back Criterion
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal criterion: %v", err)
	}
	if back.Sustain == nil || *back.Sustain != *c.Sustain {
		t.Errorf("sustain read back = %+v, want %+v", back.Sustain, c.Sustain)
	}

	// budget 0 is a real budget, "no not_met at all", so it is written.
	zero, err := json.Marshal(Criterion{ID: "strict", Description: "strict", Sustain: &Sustain{Window: "28d"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var strict struct {
		Sustain map[string]any `json:"sustain"`
	}
	if err := json.Unmarshal(zero, &strict); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b, ok := strict.Sustain["budget"]; !ok || b != float64(0) {
		t.Errorf("sustain on the wire = %v, want budget 0 written", strict.Sustain)
	}

	plain, err := json.Marshal(Criterion{ID: "plain", Description: "plain"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(plain, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := keys["sustain"]; ok {
		t.Errorf("a criterion without sustain wrote %s", plain)
	}
}
