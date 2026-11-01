package objective

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Sustain is how a standing objective's criterion holds over time: a budget
// over a rolling window, not a streak. The criterion is sustained while the
// share of settled passes in the window that ended not_met stays within
// Budget. Below MinPasses settled passes it is inconclusive: too_few_samples,
// because with three passes one failure is a third of them. Reconcile reads
// it; the loop still settles each pass once and converges (AGENTS.md rule 8).
type Sustain struct {
	// Window is the rolling window, a number of days such as "28d" or a Go
	// duration such as "72h". It is never a calendar period.
	Window string `json:"window"`
	// Budget is the share of the window's settled passes that may end
	// not_met, from 0 to 1.
	Budget float64 `json:"budget"`
	// MinPasses is how many settled passes the window must hold before the
	// budget says anything.
	MinPasses int `json:"min_passes"`
}

// CriterionPass is one reconcile pass's settlement of a criterion, with the
// time of the pass. State was settled against the criterion's own target, so
// sustain never compares a pass with earlier passes' values.
type CriterionPass struct {
	// CriterionID names the criterion the pass settled, so the passes one
	// reconcile outcome records can be told apart. Evaluate reads one
	// criterion's passes and ignores it.
	CriterionID string             `json:"criterion_id,omitempty"`
	At          time.Time          `json:"at"`
	State       CriterionState     `json:"state"`
	Reason      InconclusiveReason `json:"reason,omitempty"`
}

// SustainResult is what Evaluate made of the passes in the window. Settled
// and NotMet leave inconclusive passes out. BudgetLeft is Budget less the
// share NotMet/Settled already spent, never below zero, and the whole Budget
// when nothing settled. Latest is the latest pass's state by time, shown beside
// it so a criterion that recovered after spending budget can be seen.
type SustainResult struct {
	State      CriterionState
	Reason     InconclusiveReason
	Settled    int
	NotMet     int
	BudgetLeft float64
	Latest     CriterionState
}

// maxWindowDays is the most whole days a time.Duration can hold.
const maxWindowDays = int64(math.MaxInt64 / int64(24*time.Hour))

// ParseWindow reads a sustain window: a whole number of days such as "28d",
// or anything time.ParseDuration reads, such as "72h". A window no longer
// than zero could never hold a pass, so it is refused as well.
func ParseWindow(s string) (time.Duration, error) {
	window, err := parseWindowLength(s)
	if err != nil {
		return 0, err
	}
	if window <= 0 {
		return 0, fmt.Errorf("window %q must be longer than zero", s)
	}
	return window, nil
}

func parseWindowLength(s string) (time.Duration, error) {
	days, inDays := strings.CutSuffix(s, "d")
	if !inDays {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("window %q is neither a whole number of days such as 28d nor a duration such as 72h", s)
		}
		return d, nil
	}
	// Digits only, so neither a sign, a space nor a fraction slips past
	// strconv.
	if days == "" || strings.Trim(days, "0123456789") != "" {
		return 0, fmt.Errorf("window %q: a window in days is a whole number followed by d, such as 28d", s)
	}
	n, err := strconv.ParseInt(days, 10, 64)
	if err != nil || n > maxWindowDays {
		return 0, fmt.Errorf("window %q is longer than a window can be", s)
	}
	return time.Duration(n) * 24 * time.Hour, nil
}

// Evaluate settles the sustain state from the passes in the rolling window
// [max(now-window, since), now]. since is when the window was last restarted,
// as by a person's target change; zero is never.
//
// A pass that ended inconclusive spends no budget and is left out of both
// counts, so it neither dilutes the budget nor counts towards MinPasses; it can
// still be the latest pass. A window Evaluate cannot read settles nothing:
// create refuses one, and should one reach here it is inconclusive,
// unsupported, never quietly sustained (AGENTS.md rule 10).
func (s Sustain) Evaluate(passes []CriterionPass, now, since time.Time) SustainResult {
	result := SustainResult{BudgetLeft: s.Budget}
	window, err := ParseWindow(s.Window)
	if err != nil {
		result.State, result.Reason = CriterionInconclusive, ReasonUnsupported
		return result
	}
	start := now.Add(-window)
	if since.After(start) {
		start = since
	}

	var latest time.Time
	for _, p := range passes {
		if p.At.Before(start) || p.At.After(now) {
			continue
		}
		if result.Latest == "" || !p.At.Before(latest) {
			latest, result.Latest = p.At, p.State
		}
		switch p.State {
		case CriterionMet:
			result.Settled++
		case CriterionNotMet:
			result.Settled++
			result.NotMet++
		}
	}

	if result.Settled > 0 {
		// A division, so a share the budget names exactly, such as 1 in 10
		// against 0.1, rounds to the same float the budget does.
		spent := float64(result.NotMet) / float64(result.Settled)
		result.BudgetLeft = max(s.Budget-spent, 0)
		result.State = CriterionMet
		if spent > s.Budget {
			result.State = CriterionNotMet
		}
	}
	if result.Settled == 0 || result.Settled < s.MinPasses {
		result.State, result.Reason = CriterionInconclusive, ReasonTooFewSamples
	}
	return result
}
