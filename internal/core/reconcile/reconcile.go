// Package reconcile holds the runtime state of the outer control loop: what a
// standing objective's world looked like when it last converged, when it is
// next due, who is currently working on it, and how it has been going.
//
// The split against internal/core/objective is deliberate and follows the one
// internal/core/loop already draws. What an operator declares — mode, cadence,
// autonomy — lives on the Objective, because it is part of the objective's
// description and is edited by the person who wrote it. What the system
// discovers by running lives here, because it is written by the supervisor and
// read by nobody who is allowed to edit it.
package reconcile

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// Trigger is why a reconcile ran. It is recorded on every outcome because
// "this cost eleven dollars" is a different conversation depending on whether
// something changed, a clock struck, or a person clicked a button.
type Trigger string

const (
	// TriggerSchedule is the cadence's wall-clock or interval schedule
	// coming due. It reconciles whether or not anything drifted.
	TriggerSchedule Trigger = "schedule"

	// TriggerDrift is the cheap sense tier finding the world no longer
	// matches the fingerprint taken at the last convergence.
	TriggerDrift Trigger = "drift"

	// TriggerResync is the periodic full re-verification that runs even
	// with a still fingerprint, for the criteria a hash cannot see.
	TriggerResync Trigger = "resync"

	// TriggerManual is a person or an API client asking directly.
	TriggerManual Trigger = "manual"

	// TriggerEvent is an external signal — today, a checkpoint resolving
	// on an objective that was waiting for one.
	TriggerEvent Trigger = "event"
)

// Phase is what the supervisor is doing with an objective right now.
type Phase string

const (
	// PhaseIdle means nothing is running; NextDueAt says when that changes.
	PhaseIdle Phase = "idle"
	// PhaseSensing means the cheap tier is running.
	PhaseSensing Phase = "sensing"
	// PhaseReconciling means a loop is running.
	PhaseReconciling Phase = "reconciling"
	// PhaseWaiting means a loop escalated and is waiting on a human.
	PhaseWaiting Phase = "waiting"
	// PhasePaused means an operator stopped it, or the circuit breaker did.
	PhasePaused Phase = "paused"
)

// Fingerprint is a composite hash of every environment snapshot an objective
// observes, and the whole basis of the cheap tier: equal fingerprints mean
// nothing the objective can see has moved, so there is nothing to converge and
// no reason to spend a model call finding that out.
//
// An environment whose Snapshot carries no SHA contributes nothing rather than
// contributing an empty string. That is the honest reading — a calendar
// adapter that cannot hash its own state is saying "I don't know", and a
// system that treated "I don't know" as "unchanged" would go quiet exactly
// when it should not. Objectives over such environments are driven by their
// schedule instead, and Drift.Blind reports that this is what is happening.
type Fingerprint struct {
	// SHA is the composite hash, empty when no environment could be hashed.
	SHA string `json:"sha,omitempty"`
	// Environments maps environment ID to that environment's own SHA, kept
	// so a drift report can name what moved rather than only that something
	// did.
	Environments map[string]string `json:"environments,omitempty"`
	// Blind lists environments that returned no SHA.
	Blind []string `json:"blind,omitempty"`
	// TakenAt is when the snapshots were read.
	TakenAt time.Time `json:"taken_at"`
}

// Drift is the result of comparing a fresh fingerprint against the one taken
// at the last convergence.
type Drift struct {
	// Changed is true when at least one environment's SHA moved.
	Changed bool `json:"changed"`
	// Environments names the environments whose SHA moved.
	Environments []string `json:"environments,omitempty"`
	// From and To are the composite hashes either side of the comparison.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Blind is true when nothing could be hashed at all, so the comparison
	// proves nothing. A blind sense is never reported as drift; it is
	// reported as blind, and the schedule is what moves the objective.
	Blind bool `json:"blind"`
}

// Outcome is one completed pass of the outer loop, cheap or expensive.
type Outcome struct {
	ID          string                `json:"id"`
	ObjectiveID objective.ObjectiveID `json:"objective_id"`
	TwinID      string                `json:"twin_id,omitempty"`

	Trigger Trigger `json:"trigger"`
	// LoopID is empty when the pass sensed and stopped — which is the
	// common case, and the one the design is built to make cheap.
	LoopID string `json:"loop_id,omitempty"`

	Drift Drift `json:"drift"`
	// Autonomy is the level the pass ran at, after the ceiling was applied.
	Autonomy objective.AutonomyLevel `json:"autonomy,omitempty"`

	// CriteriaMet is the weighted score the loop reached, and Converged is
	// whether it reached 1.0. A sense-only pass carries the previous score
	// and reports Converged from the state it did not change.
	CriteriaMet float64 `json:"criteria_met"`
	Converged   bool    `json:"converged"`

	// Escalated is true when the pass ended waiting on a human.
	Escalated    bool   `json:"escalated"`
	CheckpointID string `json:"checkpoint_id,omitempty"`

	// Error is why the pass failed, empty when it did not.
	Error string `json:"error,omitempty"`

	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// Failed reports whether this pass counts against the circuit breaker.
//
// An escalation is not a failure. A loop that stopped to ask a question did
// the right thing, and a breaker that counted questions would trip precisely
// on the objectives being most careful.
func (o Outcome) Failed() bool { return o.Error != "" }

// State is the durable record of one standing objective's outer loop — the
// supervisor's equivalent of loop.State, and persisted for the same reason:
// the process holding it can die, and the work has to survive that.
//
// It is also the lease. Holder and LeaseUntil are what stop two replicas
// reconciling the same objective, sending the same mail and paying twice. The
// claim is a conditional UPDATE, so the database arbitrates rather than a
// coordination service Karakuri does not have.
type State struct {
	ObjectiveID objective.ObjectiveID `json:"objective_id"`
	TwinID      string                `json:"twin_id,omitempty"`

	Phase Phase `json:"phase"`
	// Paused is an operator's or the breaker's stop. It survives restarts,
	// which is the point: a breaker that forgot it had tripped would put
	// the failing objective straight back into the rotation.
	Paused       bool   `json:"paused"`
	PausedReason string `json:"paused_reason,omitempty"`

	// NextDueAt is the single value the supervisor's due-wheel queries on.
	// Everything the cadence expresses — schedule, sense interval, resync,
	// min-interval floor, quiet windows, backoff — is resolved into this
	// one timestamp, so the hot path is one indexed comparison rather than
	// cron arithmetic across every objective on every tick.
	//
	// Nil means never due on its own, which is a real state and not the
	// same thing as due since the epoch: a standing objective may declare
	// no schedule at all and reconcile only when somebody asks.
	NextDueAt *time.Time `json:"next_due_at,omitempty"`
	// NextSenseAt and NextReconcileAt are kept alongside so the API can
	// explain which of the two produced NextDueAt without recomputing it.
	NextSenseAt     *time.Time `json:"next_sense_at,omitempty"`
	NextReconcileAt *time.Time `json:"next_reconcile_at,omitempty"`

	// Converged records the last fingerprint at which the criteria were
	// met. Drift is measured against this rather than against the previous
	// sense, so an environment that flaps away and back is not drift.
	Converged       Fingerprint `json:"converged_fingerprint"`
	LastConvergedAt *time.Time  `json:"last_converged_at,omitempty"`

	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
	LastTrigger      Trigger    `json:"last_trigger,omitempty"`
	LastOutcomeID    string     `json:"last_outcome_id,omitempty"`
	LastError        string     `json:"last_error,omitempty"`

	// ActiveLoopID is the loop this objective left running — set when a
	// reconcile escalated and the supervisor stopped waiting on it.
	//
	// It has to be here rather than inferred, because the supervisor
	// deliberately does not sit on a paused loop: a reconcile that stopped to
	// ask a human could wait days, and holding a lease and a concurrency slot
	// for that long would starve every other standing objective. Letting go
	// means remembering what was let go of.
	ActiveLoopID string `json:"active_loop_id,omitempty"`

	// CriteriaMet is the most recent weighted score, and ScoreStreak counts
	// consecutive reconciles that failed to improve it. The streak is the
	// stall detector: an objective whose score has not moved in three
	// expensive runs is not converging, and running it a fourth time is
	// buying nothing.
	CriteriaMet float64 `json:"criteria_met"`
	ScoreStreak int     `json:"score_streak"`

	// ConsecutiveFailures drives the circuit breaker and the backoff.
	ConsecutiveFailures int `json:"consecutive_failures"`

	// Autonomy is the level the objective has earned, always within the
	// ceiling its declaration set. CleanRuns counts consecutive reconciles
	// with no rejection and no failure, and resets on either.
	Autonomy  objective.AutonomyLevel `json:"autonomy,omitempty"`
	CleanRuns int                     `json:"clean_runs"`

	// Holder is the server instance currently working on this objective and
	// LeaseUntil is when that claim expires. A crashed holder's lease runs
	// out and another replica picks the objective up; it does not need to
	// have released anything.
	Holder     string     `json:"holder,omitempty"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EffectiveAutonomy is the level this objective may run at right now: what it
// has earned, confined to what its declaration allows. A state whose stored
// level was edited, corrupted or written by an older build cannot escape the
// ceiling, because the ceiling is applied here on read rather than trusted
// from the row.
func (s State) EffectiveAutonomy(decl objective.Autonomy) objective.AutonomyLevel {
	if s.Autonomy == "" {
		return decl.Clamp(decl.EffectiveLevel())
	}
	return decl.Clamp(s.Autonomy)
}

// Leased reports whether somebody else holds a live claim on this objective.
func (s State) Leased(now time.Time, me string) bool {
	if s.LeaseUntil == nil || s.Holder == "" {
		return false
	}
	if s.Holder == me {
		return false
	}
	return s.LeaseUntil.After(now)
}
