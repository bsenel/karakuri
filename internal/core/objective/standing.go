package objective

import "time"

// Mode says whether an objective finishes or is held.
//
// The zero value is oneshot, which is what every objective written before
// standing objectives existed deserializes as. That is deliberate: adding a
// column must not change what an existing row means.
type Mode string

const (
	// ModeOneshot converges once and stops. The loop runs to a met criteria
	// score or its iteration cap, the objective goes to completed or failed,
	// and nothing runs again unless somebody starts another loop.
	ModeOneshot Mode = "oneshot"

	// ModeStanding declares a desired state to be held rather than a task to
	// be finished. A supervisor senses drift on a cadence and reconciles when
	// the world has moved away from what the success criteria describe. A
	// standing objective has no terminal success state — converged is a
	// resting place, not an end.
	ModeStanding Mode = "standing"
)

// IsStanding reports whether this objective is held rather than finished.
func (o Objective) IsStanding() bool { return o.Mode == ModeStanding }

// Cadence declares when a standing objective is checked and when it is
// reconciled. The two are deliberately separate settings, because they cost
// wildly different amounts.
//
// Sensing calls Snapshot on each environment and compares a composite hash
// against the last converged one: a handful of adapter calls, no model call,
// no tokens. Reconciling runs the full observe→learn loop, which is where the
// money goes. An objective can therefore sense every fifteen minutes all year
// and only spend on the days something actually moved.
//
// Durations are strings parsed with time.ParseDuration, matching how the
// config file spells TTLs ("15m", "720h"). The parsing itself lives in
// internal/platform/schedule, which owns the cron dependency; this package
// stays a description of what was declared.
type Cadence struct {
	// Sense is how often to check cheaply for drift. Empty or zero means
	// never — the objective is then driven purely by Every/Cron/Resync,
	// which is the right shape for environments whose Snapshot carries no
	// meaningful hash (a calendar, an inbox).
	Sense string `json:"sense,omitempty"`

	// Every reconciles unconditionally on a fixed interval. Mutually
	// exclusive with Cron and DailyAt.
	Every string `json:"every,omitempty"`

	// Cron is a five-field expression (minute hour day-of-month month
	// day-of-week) evaluated in Timezone. Mutually exclusive with Every.
	Cron string `json:"cron,omitempty"`

	// DailyAt is "HH:MM" in Timezone — the common case spelled without a
	// cron expression. Mutually exclusive with Every and Cron.
	DailyAt string `json:"daily_at,omitempty"`

	// Timezone is an IANA name. Empty means UTC. It matters more than it
	// looks: "the 8am report" is a local-clock promise, and a schedule
	// pinned to UTC drifts an hour twice a year against the person reading
	// it.
	Timezone string `json:"timezone,omitempty"`

	// Resync forces a reconcile this long after the last one even when
	// nothing drifted. Kubernetes learned this the hard way: a hash that
	// did not move is evidence about the things the hash covers and about
	// nothing else. A deadline approaching, an expiring credential, a
	// criterion whose verifier reads a clock — none of those move a
	// snapshot SHA.
	Resync string `json:"resync,omitempty"`

	// MinInterval is a floor between reconciles regardless of what asked
	// for one. It is the anti-thrash guard: an environment that changes
	// every few seconds must not be able to drive a loop that costs money
	// every few seconds.
	MinInterval string `json:"min_interval,omitempty"`

	// Quiet lists blackout windows as "HH:MM-HH:MM" in Timezone. A window
	// may wrap midnight. Work due inside one is deferred to the next open
	// moment, never dropped — a report that would have gone out at 3am
	// arrives at 7am rather than not at all.
	Quiet []string `json:"quiet,omitempty"`
}

// SenseInterval, ReconcileInterval, ResyncInterval and MinInterval parse their
// respective fields, returning zero for anything empty or unparseable. The
// caller decides what zero means; schedule.Validate is what rejects a
// malformed cadence at declaration time, so a zero here is an absent setting
// rather than a silent typo.
func (c Cadence) SenseInterval() time.Duration     { return parseDuration(c.Sense) }
func (c Cadence) ReconcileInterval() time.Duration { return parseDuration(c.Every) }
func (c Cadence) ResyncInterval() time.Duration    { return parseDuration(c.Resync) }
func (c Cadence) MinIntervalDuration() time.Duration {
	return parseDuration(c.MinInterval)
}

// HasSchedule reports whether the cadence names a wall-clock schedule as
// opposed to a plain interval.
func (c Cadence) HasSchedule() bool { return c.Cron != "" || c.DailyAt != "" }

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

// AutonomyLevel is how much a standing objective may do without asking.
//
// The levels are a ladder rather than a set of flags because the question an
// operator is actually answering is "how far do I trust this yet", and that
// has an order. They map onto agent.AuthorityBounds, which already exists and
// already gates the decide step — no second enforcement path is introduced.
type AutonomyLevel string

const (
	// AutonomySense never reconciles. It senses, records drift and reports
	// it. It cannot spend tokens, which makes it the honest setting for a
	// new objective nobody has watched yet.
	AutonomySense AutonomyLevel = "sense"

	// AutonomyPropose plans but never acts: every action escalates to a
	// checkpoint carrying the draft. This is the default.
	AutonomyPropose AutonomyLevel = "propose"

	// AutonomyActWithNotice honours the agent definition's own bounds and
	// names every autonomous action in the next digest.
	AutonomyActWithNotice AutonomyLevel = "act_with_notice"

	// AutonomyAct honours the agent's bounds and reports exceptions only.
	AutonomyAct AutonomyLevel = "act"
)

var autonomyLadder = []AutonomyLevel{
	AutonomySense,
	AutonomyPropose,
	AutonomyActWithNotice,
	AutonomyAct,
}

// Rank returns the level's position on the ladder, 0 for sense through 3 for
// act. An unrecognised level ranks as propose rather than as act: a typo in a
// stored declaration must fail toward asking, never toward acting.
func (l AutonomyLevel) Rank() int {
	for i, known := range autonomyLadder {
		if known == l {
			return i
		}
	}
	return 1
}

// Valid reports whether the level is one of the four known rungs.
func (l AutonomyLevel) Valid() bool {
	for _, known := range autonomyLadder {
		if known == l {
			return true
		}
	}
	return false
}

// AutonomyByRank returns the level at a ladder position, clamped to the ends.
func AutonomyByRank(rank int) AutonomyLevel {
	if rank < 0 {
		rank = 0
	}
	if rank >= len(autonomyLadder) {
		rank = len(autonomyLadder) - 1
	}
	return autonomyLadder[rank]
}

// Autonomy declares how much latitude a standing objective starts with, how
// much it may ever earn, and what it takes to move.
//
// Ceiling is the load-bearing field. Karakuri may promote itself toward it and
// never past it, so the worst case of a trust ledger gone wrong is the level a
// human already wrote down. Demotion needs no counter: one rejection drops a
// rung immediately, because a reviewer saying no is a stronger signal than any
// number of runs nobody objected to.
type Autonomy struct {
	// Level is where the objective starts. Empty means propose.
	Level AutonomyLevel `json:"level,omitempty"`

	// Ceiling is the highest level this objective may ever reach. Empty
	// means it never rises above Level.
	Ceiling AutonomyLevel `json:"ceiling,omitempty"`

	// PromoteAfter is how many consecutive clean reconciles earn one rung.
	// Zero disables promotion, which is what an operator who wants a fixed
	// level leaves it at.
	PromoteAfter int `json:"promote_after,omitempty"`

	// DemoteOnFailure additionally demotes on a failed reconcile, not only
	// on a rejected checkpoint. Off by default: a flaky environment is a
	// reason to fix the environment, not to distrust the agent.
	DemoteOnFailure bool `json:"demote_on_failure,omitempty"`
}

// EffectiveLevel is the declared starting level, defaulting to propose.
func (a Autonomy) EffectiveLevel() AutonomyLevel {
	if a.Level.Valid() {
		return a.Level
	}
	return AutonomyPropose
}

// EffectiveCeiling is the highest reachable level. An unset ceiling pins the
// objective at its starting level, and a ceiling below that starting level
// wins — the ceiling is a bound, so it binds in both directions.
func (a Autonomy) EffectiveCeiling() AutonomyLevel {
	if !a.Ceiling.Valid() {
		return a.EffectiveLevel()
	}
	return a.Ceiling
}

// Clamp confines a level to this declaration's ceiling and to the bottom of
// the ladder. Every path that decides what an objective may do runs through
// here, so there is one place where the ceiling is enforced.
func (a Autonomy) Clamp(l AutonomyLevel) AutonomyLevel {
	ceiling := a.EffectiveCeiling()
	if !l.Valid() {
		return a.EffectiveLevel()
	}
	if l.Rank() > ceiling.Rank() {
		return ceiling
	}
	return l
}
