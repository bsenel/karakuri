// Package digest is the shape of a periodic report: what a twin's standing
// objectives did over a window, and what they need a person to decide.
//
// It is a value assembled from records that already exist — reconcile
// outcomes, the audit log, pending checkpoints, the cost ledger — rather than
// a new thing written as work happens. Nothing here is a second source of
// truth, so a digest can be regenerated for any past window and will say the
// same thing.
package digest

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// Digest is one report over one twin and one window.
type Digest struct {
	TwinID   string    `json:"twin_id"`
	TwinName string    `json:"twin_name,omitempty"`
	Since    time.Time `json:"since"`
	Until    time.Time `json:"until"`

	Objectives []ObjectiveSummary `json:"objectives,omitempty"`

	// Decisions is what the report exists for. Everything above it is
	// context; this is the part the reader has to act on, and it is why the
	// digest is delivered at all rather than left in a console somebody
	// might open.
	Decisions []Decision `json:"decisions,omitempty"`

	Spend Spend `json:"spend"`

	// AutonomyChanges records objectives that earned or lost a rung in the
	// window. Surfaced separately from their objective's summary because a
	// change in what Karakuri may do without asking is the one line in a
	// report that should never be scrolled past.
	AutonomyChanges []AutonomyChange `json:"autonomy_changes,omitempty"`

	// Prose is the rendered narrative. Empty when no agent was available to
	// write one, in which case the structured fields are delivered on their
	// own rather than nothing being sent.
	Prose string `json:"prose,omitempty"`
}

// Empty reports whether there is nothing worth sending.
//
// A digest with no activity, no decisions and no spend is a mail that says
// "nothing happened", and a daily mail that says nothing happened is a mail
// people stop reading — which costs the ones that matter their audience.
func (d Digest) Empty() bool {
	return len(d.Decisions) == 0 &&
		len(d.AutonomyChanges) == 0 &&
		d.Spend.Cost == 0 &&
		!d.anyActivity()
}

func (d Digest) anyActivity() bool {
	for _, o := range d.Objectives {
		if o.Reconciles > 0 || o.Failures > 0 || o.Actions > 0 || o.DriftDetected > 0 {
			return true
		}
	}
	return false
}

// ObjectiveSummary is one standing objective's window.
type ObjectiveSummary struct {
	ID     objective.ObjectiveID     `json:"id"`
	Title  string                    `json:"title"`
	Status objective.ObjectiveStatus `json:"status"`

	// Senses counts the cheap passes and Reconciles the expensive ones.
	// Reported separately and always, because the ratio between them is the
	// answer to "is this thing costing me anything", and a report that
	// showed only the expensive passes would make a well-behaved objective
	// look like an idle one.
	Senses     int `json:"senses"`
	Reconciles int `json:"reconciles"`

	DriftDetected int `json:"drift_detected"`
	Converged     int `json:"converged"`
	Escalations   int `json:"escalations"`
	Failures      int `json:"failures"`

	// Actions counts what it did to the world, from the audit log rather
	// than from the outcome — the outcome says a loop ran, the audit says
	// what the loop touched.
	Actions int `json:"actions"`

	Autonomy    objective.AutonomyLevel `json:"autonomy,omitempty"`
	CriteriaMet float64                 `json:"criteria_met"`
	Paused      bool                    `json:"paused,omitempty"`
	PausedWhy   string                  `json:"paused_why,omitempty"`
	LastError   string                  `json:"last_error,omitempty"`
}

// Decision is a pending checkpoint, rendered as the question it is.
type Decision struct {
	CheckpointID   string                `json:"checkpoint_id"`
	ObjectiveID    objective.ObjectiveID `json:"objective_id"`
	ObjectiveTitle string                `json:"objective_title,omitempty"`
	Reason         string                `json:"reason,omitempty"`
	Summary        string                `json:"summary"`
	Options        []string              `json:"options,omitempty"`
	// Proposed lists the capabilities the agent wanted to run, so the reader
	// can judge what they are approving without opening anything.
	Proposed  []string  `json:"proposed,omitempty"`
	WaitingAt time.Time `json:"waiting_since"`
}

// Age is how long the decision has been waiting at the time the digest was
// built. A checkpoint that has been pending for three days is a different
// message from one raised an hour ago, and the reader should not have to
// subtract dates to notice.
func (d Decision) Age(now time.Time) time.Duration { return now.Sub(d.WaitingAt) }

// AutonomyChange is one movement up or down the ladder.
type AutonomyChange struct {
	ObjectiveID    objective.ObjectiveID   `json:"objective_id"`
	ObjectiveTitle string                  `json:"objective_title,omitempty"`
	From           objective.AutonomyLevel `json:"from"`
	To             objective.AutonomyLevel `json:"to"`
	Reason         string                  `json:"reason,omitempty"`
	At             time.Time               `json:"at"`
}

// Promoted reports whether the change widened what the objective may do.
func (a AutonomyChange) Promoted() bool { return a.To.Rank() > a.From.Rank() }

// Spend is what the window cost, by provider.
//
// Zero is a real answer and is reported as one: a deployment with no price
// table configured counts units and prices nothing, and a report that
// silently omitted the line would read as "this was free".
type Spend struct {
	Cost       float64            `json:"cost"`
	ByProvider map[string]float64 `json:"by_provider,omitempty"`
	// Priced is false when no rate table is configured, so a zero above
	// means "not priced" rather than "nothing was spent".
	Priced bool `json:"priced"`
}
