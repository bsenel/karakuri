package reconcile

import (
	"strings"
	"time"
)

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
	Attempts int `json:"attempts,omitempty"`
	// Tried is each failed attempt, oldest first (Phase 40 step 14c).
	Tried     []Attempt `json:"tried,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// Attempt is one diagnose-and-fix pass on a blocker that read its signal
// still raised afterwards (Phase 40 step 14c): what the pass found, what it
// tried, and the audit rows the pass wrote.
type Attempt struct {
	LoopID    string    `json:"loop_id"`
	At        time.Time `json:"at"`
	Diagnosis string    `json:"diagnosis,omitempty"`
	Remedy    string    `json:"remedy,omitempty"`
	AuditRows []string  `json:"audit_rows,omitempty"`
}

// HandoverReason is why a blocker was handed to the engineering backlog.
type HandoverReason string

const (
	// HandoverAttemptsSpent is a blocker the stream tried the declared number
	// of times without clearing its signal; the stream stops trying.
	HandoverAttemptsSpent HandoverReason = "attempts_spent"
	// HandoverRecurring is a kind cleared for the second time inside the
	// declared window; the stream keeps running.
	HandoverRecurring HandoverReason = "recurring"
)

// Handover is a blocker the stream cannot remove, or one that comes back,
// listed for the engineering_backlog stream to file as one entry per kind
// (Phase 40 step 14c). Later occurrences add evidence to it.
type Handover struct {
	Kind      BlockerKind    `json:"kind"`
	Subject   string         `json:"subject"`
	Reason    HandoverReason `json:"reason"`
	Diagnosis string         `json:"diagnosis,omitempty"`
	Attempts  []Attempt      `json:"attempts,omitempty"`
	// Cleared is when each occurrence of a recurring kind was cleared.
	Cleared []time.Time `json:"cleared,omitempty"`
	At      time.Time   `json:"at"`
}

// Clear is one blocker a pass read cleared, kept only until its kind is
// handed over as recurring or the declared window has passed it by.
type Clear struct {
	Kind    BlockerKind `json:"kind"`
	Subject string      `json:"subject"`
	At      time.Time   `json:"at"`
}

// BlockedOn is how a stream that spent its attempts on a blocker reads in the
// oversight view and in its paused reason: "blocked on <diagnosis>, tried
// <what>", each remedy once, in the order it was tried.
func BlockedOn(diagnosis string, tried []Attempt) string {
	seen := map[string]bool{}
	var remedies []string
	for _, a := range tried {
		if a.Remedy == "" || seen[a.Remedy] {
			continue
		}
		seen[a.Remedy] = true
		remedies = append(remedies, a.Remedy)
	}
	if diagnosis == "" {
		diagnosis = "an undiagnosed blocker"
	}
	what := "nothing named"
	if len(remedies) > 0 {
		what = strings.Join(remedies, "; ")
	}
	return "blocked on " + diagnosis + ", tried " + what
}

// LastDiagnosis is the newest diagnosis among the attempts, empty when none
// named one.
func LastDiagnosis(tried []Attempt) string {
	for i := len(tried) - 1; i >= 0; i-- {
		if tried[i].Diagnosis != "" {
			return tried[i].Diagnosis
		}
	}
	return ""
}
