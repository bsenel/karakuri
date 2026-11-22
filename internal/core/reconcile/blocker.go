package reconcile

import "time"

// BlockerKind is why a stream's pass ended without progress for a reason that
// is not "nothing to do" (Phase 40 step 14a).
type BlockerKind string

const (
	BlockerVerifyFailed        BlockerKind = "verify_failed"
	BlockerPlanRefused         BlockerKind = "plan_refused"
	BlockerPlanError           BlockerKind = "plan_error"
	BlockerUnmergeablePR       BlockerKind = "unmergeable_pr"
	BlockerDeferredQuota       BlockerKind = "deferred_quota"
	BlockerPushedRefsViolation BlockerKind = "pushed_refs_violation"
	BlockerNoProgress          BlockerKind = "no_progress"
)

// Blocker is one thing a stream is stuck on. Its identity is (Kind, Subject),
// so the same blocker seen on a later pass is counted, not listed twice.
type Blocker struct {
	Kind BlockerKind `json:"kind"`
	// Subject is the pull request, branch or item the blocker is about.
	Subject string `json:"subject"`
	Count   int    `json:"count"`
	// Attempts is how many diagnose-and-fix passes tried a remedy for this
	// blocker and read its signal still raised (Phase 40 step 14b). A blocker
	// that clears leaves the list, so its next occurrence starts at zero.
	Attempts  int       `json:"attempts,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}
