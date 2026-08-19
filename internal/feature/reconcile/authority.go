package reconcile

import (
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// effectiveAuthority renders an autonomy level into the bounds the loop
// already enforces.
//
// This is the hinge of the whole design, and it is a pure function over a
// struct the decide step has been reading since Phase 1. There is no second
// gate, no parallel policy, and no change to the six steps: the supervisor
// decides how far it is trusted, writes that into the request, and the loop
// enforces it exactly as it enforces an operator's hand-written bounds.
//
// A second enforcement path was the obvious alternative and would have been a
// mistake. Two places that decide whether an action may run are two places
// that can disagree, and the one that disagrees quietly is the one that lets
// something through.
func effectiveAuthority(def agent.Definition, level objective.AutonomyLevel) agent.AuthorityBounds {
	bounds := def.Authority

	switch level {
	case objective.AutonomySense:
		// Never reaches the loop at all — the supervisor stops after
		// sensing. The bounds are pinned shut anyway so that a future
		// caller which does run a loop at this level cannot act by
		// accident.
		bounds.MaxAutonomousActions = 0
		bounds.ConfidenceThreshold = confidenceAlwaysEscalates
		bounds.CanDelegate = false
		bounds.CanModifyObjective = false

	case objective.AutonomyPropose:
		// Plans, then escalates whatever it planned. Zero autonomous
		// actions is what the decide step reads as "draft and ask"; the
		// threshold above any attainable confidence closes the other
		// door, since an operator resolving a checkpoint with modify can
		// lower the threshold for that iteration, and the count is what
		// still holds the line when they do.
		bounds.MaxAutonomousActions = 0
		bounds.ConfidenceThreshold = confidenceAlwaysEscalates
		bounds.CanModifyObjective = false

	case objective.AutonomyActWithNotice, objective.AutonomyAct:
		// The agent definition's own bounds apply, untouched. An operator
		// who wrote MaxAutonomousActions: 2 on an agent meant it, and a
		// supervisor that widened it because the objective had behaved
		// well for a week would be overruling the wrong person. What the
		// two levels differ in is how loudly the result is reported, which
		// is a reporting decision and lives with the digest.

	default:
		// An unrecognised level is a typo or a hand-edited row. Fail toward
		// asking.
		bounds.MaxAutonomousActions = 0
		bounds.ConfidenceThreshold = confidenceAlwaysEscalates
		bounds.CanDelegate = false
		bounds.CanModifyObjective = false
	}

	return bounds
}

// confidenceAlwaysEscalates is above the 0..1 range a plan's confidence lives
// in, so no plan can ever clear it. 1.0 would be cleared by a model that
// reported perfect confidence, which models do.
const confidenceAlwaysEscalates = 1.01

// promote returns the level an objective has earned after a clean run, and
// whether it moved.
//
// Promotion is one rung at a time and never past the declared ceiling. Both
// halves matter: a ladder that could be climbed two rungs at once would let a
// quiet week hand an objective the authority to act unsupervised, and a ladder
// with no ceiling would eventually hand it everything.
func promote(decl objective.Autonomy, current objective.AutonomyLevel, cleanRuns int) (objective.AutonomyLevel, bool) {
	if decl.PromoteAfter <= 0 || cleanRuns < decl.PromoteAfter {
		return current, false
	}
	next := decl.Clamp(objective.AutonomyByRank(current.Rank() + 1))
	if next == current {
		return current, false
	}
	return next, true
}

// demote drops one rung, and never below the bottom of the ladder.
//
// It needs no counter. A reviewer saying no is a stronger signal than any
// number of runs nobody objected to, so one rejection moves the level
// immediately while promotion takes a declared number of clean runs. The
// asymmetry is the point.
func demote(decl objective.Autonomy, current objective.AutonomyLevel) (objective.AutonomyLevel, bool) {
	next := decl.Clamp(objective.AutonomyByRank(current.Rank() - 1))
	if next == current {
		return current, false
	}
	return next, true
}
