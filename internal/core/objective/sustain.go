package objective

import (
	"errors"
	"time"
)

// errSustainNotImplemented is part A's scaffolding: the tests specify what
// Evaluate and ParseWindow do before part B writes them (Phase 41 step 8).
var errSustainNotImplemented = errors.New("sustain: not implemented")

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
	At     time.Time          `json:"at"`
	State  CriterionState     `json:"state"`
	Reason InconclusiveReason `json:"reason,omitempty"`
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

// ParseWindow reads a sustain window: a whole number of days such as "28d",
// or anything time.ParseDuration reads, such as "72h".
func ParseWindow(s string) (time.Duration, error) {
	return 0, errSustainNotImplemented
}

// Evaluate settles the sustain state from the passes in the rolling window
// [max(now-window, since), now]. since is when the window was last restarted,
// as by a person's target change; zero is never.
func (s Sustain) Evaluate(passes []CriterionPass, now, since time.Time) SustainResult {
	return SustainResult{}
}
