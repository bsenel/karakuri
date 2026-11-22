package reconcile

import (
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 40 step 14a: a blocked stream is known to be blocked. Detection and
// counting only; no behaviour changes, so each case also asserts the pass
// count and the breaker's count are what they were before this step.
//
// Which existing field carries each signal into reconcile (read from run.go):
//
//   - verify_failed: the loop row's Criteria, a required criterion not_met; the
//     pass's Outcome.Error is "loop ended without meeting its success criteria".
//     Subject: that criterion's ID (the item).
//   - plan_error: the loop row's Outcome == RunErrored, with Error set. The pass
//     outcome only keeps Error text, which also says "success criteria"; the
//     typed signal is the row. Subject: the objective ID.
//   - pushed_refs_violation: the loop row's BoundsViolation (step 3), also
//     State.PausedReason. Subject: its first ref.
//   - deferred_quota: Outcome.Deferred ("budget_exhausted" or
//     "provider_rate_limited"). Subject: the objective ID. Never an Error, so
//     ConsecutiveFailures does not move.
//   - no_progress: no field. The branch's tip is the "git" environment's
//     fingerprint SHA; a pass with work and an unmoved SHA, noProgressPasses
//     times running. Subject: the objective ID.
//   - plan_refused: the loop row's PlanRefusedReason, set in feature/loop/decide.go
//     where the bounds refuse a plan. Subject: the objective ID.
//   - unmergeable_pr: DOES NOT REACH RECONCILE. Nothing in this package or its
//     snapshot (domains/software's delivery_work) reads a pull request's
//     mergeability. Same failing test, same message.

func blockerFixture(t *testing.T) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Status:   objective.StatusActive,
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// again runs one more manual pass, past the cadence floor.
func (f *fixture) again(t *testing.T, id objective.ObjectiveID) {
	t.Helper()
	f.clock = f.clock.Add(2 * time.Hour)
	f.pass(t, id, reconcile.TriggerManual)
}

func (f *fixture) failVerify(ids ...string) {
	f.loops.mu.Lock()
	defer f.loops.mu.Unlock()
	f.loops.status = objective.StatusFailed
	f.loops.failure = ""
	f.loops.criteria = nil
	for _, id := range ids {
		f.loops.criteria = append(f.loops.criteria, objective.Criterion{ID: id, State: objective.CriterionNotMet})
	}
}

func wantBlockers(t *testing.T, st reconcile.State, want ...reconcile.Blocker) {
	t.Helper()
	if len(st.Blockers) != len(want) {
		t.Fatalf("blockers = %+v, want %d: %+v", st.Blockers, len(want), want)
	}
	for _, w := range want {
		found := false
		for _, b := range st.Blockers {
			if b.Kind == w.Kind && b.Subject == w.Subject {
				found = true
				if w.Count != 0 && b.Count != w.Count {
					t.Errorf("blocker %s/%s count = %d, want %d", b.Kind, b.Subject, b.Count, w.Count)
				}
				if b.FirstSeen.IsZero() || b.LastSeen.Before(b.FirstSeen) {
					t.Errorf("blocker %s/%s has first_seen %v, last_seen %v", b.Kind, b.Subject, b.FirstSeen, b.LastSeen)
				}
			}
		}
		if !found {
			t.Errorf("no blocker %s on %q in %+v", w.Kind, w.Subject, st.Blockers)
		}
	}
}

func TestEachSignalYieldsOneBlocker(t *testing.T) {
	cases := []struct {
		name string
		kind reconcile.BlockerKind
		// seed arranges the loop's answer and returns the expected subject.
		seed func(f *fixture) string
		// unreachable, when set, is why no existing field carries the signal.
		unreachable string
	}{
		{name: "failed verify", kind: reconcile.BlockerVerifyFailed, seed: func(f *fixture) string {
			f.failVerify("tests-green")
			return "tests-green"
		}},
		{name: "plan that is only an error", kind: reconcile.BlockerPlanError, seed: func(f *fixture) string {
			f.loops.status, f.loops.runOutcome, f.loops.failure = objective.StatusFailed, coreloop.RunErrored, "the model returned no plan"
			return "obj-1"
		}},
		{name: "pushed refs violation", kind: reconcile.BlockerPushedRefsViolation, seed: func(f *fixture) string {
			f.loops.status, f.loops.failure = objective.StatusFailed, "the delegated run failed"
			f.loops.violation = &environment.BoundsViolation{Reason: "main moved", Refs: []string{"refs/heads/main"}}
			return "refs/heads/main"
		}},
		{name: "provider rate limit deferral", kind: reconcile.BlockerDeferredQuota, seed: func(f *fixture) string {
			f.loops.status, f.loops.failure = objective.StatusFailed, rateLimitText()
			return "obj-1"
		}},
		{name: "plan refused by the bounds", kind: reconcile.BlockerPlanRefused, seed: func(f *fixture) string {
			f.loops.status, f.loops.failure = objective.StatusFailed, "the delegated run failed"
			f.loops.refused = "action outside the bounds' limits"
			return "obj-1"
		}},
		{name: "unmergeable pull request", kind: reconcile.BlockerUnmergeablePR,
			unreachable: "no code in reconcile or the delivery_work snapshot reads a pull request's mergeability"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unreachable != "" {
				t.Fatalf("%s: cannot be detected without a new signal: %s", tc.kind, tc.unreachable)
			}
			f, obj := blockerFixture(t)
			subject := tc.seed(f)
			f.pass(t, obj.ID, reconcile.TriggerManual)
			wantBlockers(t, f.state(t, obj.ID), reconcile.Blocker{Kind: tc.kind, Subject: subject, Count: 1})
		})
	}
}

func TestABudgetDeferralIsAQuotaBlockerAndNotAFailure(t *testing.T) {
	f, obj := budgetFixture(t, 12)
	f.pass(t, obj.ID, reconcile.TriggerManual)

	st := f.state(t, obj.ID)
	wantBlockers(t, st, reconcile.Blocker{Kind: reconcile.BlockerDeferredQuota, Subject: string(obj.ID), Count: 1})
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after a quota deferral, want 0", st.ConsecutiveFailures)
	}
	if got := f.loops.count(); got != 0 {
		t.Errorf("the loop ran %d times over budget, want 0: no pass is spent on a quota blocker", got)
	}
	if st.NextDueAt == nil || !st.NextDueAt.After(f.clock.Add(time.Hour)) {
		t.Errorf("next due = %v, want the deferral's floor, unchanged by this step", st.NextDueAt)
	}
}

func TestNothingPlannedYieldsNoBlocker(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{Cadence: &objective.Cadence{Sense: "15m"}})
	// A quiet world: every pass senses and stops, and nothing is Planned.
	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, "")
		f.clock = f.clock.Add(15 * time.Minute)
	}

	st := f.state(t, obj.ID)
	if len(st.Blockers) != 0 {
		t.Errorf("blockers = %+v for a stream with nothing to do, want none", st.Blockers)
	}
	if got := f.loops.count(); got != 0 {
		t.Errorf("the loop ran %d times, want 0", got)
	}
}

func TestTheSameSignalOnTheSameSubjectIsCounted(t *testing.T) {
	f, obj := blockerFixture(t)
	f.failVerify("tests-green")
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.again(t, obj.ID)

	st := f.state(t, obj.ID)
	wantBlockers(t, st, reconcile.Blocker{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-green", Count: 2})
	// Unchanged behaviour: both passes ran, and the breaker counted both.
	if got := f.loops.count(); got != 2 {
		t.Errorf("the loop ran %d times, want 2", got)
	}
	if st.ConsecutiveFailures != 2 {
		t.Errorf("consecutive_failures = %d, want 2: blockers must not touch the breaker", st.ConsecutiveFailures)
	}
}

func TestTheSameSignalOnTwoSubjectsIsTwoBlockers(t *testing.T) {
	f, obj := blockerFixture(t)
	f.failVerify("tests-green")
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.failVerify("lint-clean")
	f.again(t, obj.ID)

	wantBlockers(t, f.state(t, obj.ID),
		reconcile.Blocker{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-green", Count: 1},
		reconcile.Blocker{Kind: reconcile.BlockerVerifyFailed, Subject: "lint-clean", Count: 1},
	)
}

func TestAPassThatMadeProgressClearsTheBlockerOfItsSubject(t *testing.T) {
	f, obj := blockerFixture(t)
	f.failVerify("tests-green", "lint-clean")
	f.pass(t, obj.ID, reconcile.TriggerManual)

	// tests-green is now met; lint-clean is still not.
	f.failVerify("lint-clean")
	f.loops.mu.Lock()
	f.loops.criteria = append([]objective.Criterion{{ID: "tests-green", State: objective.CriterionMet}}, f.loops.criteria...)
	f.loops.mu.Unlock()
	f.again(t, obj.ID)

	wantBlockers(t, f.state(t, obj.ID), reconcile.Blocker{Kind: reconcile.BlockerVerifyFailed, Subject: "lint-clean", Count: 2})
}

func TestNoProgressIsRaisedOnlyAfterTheDeclaredNumberOfPasses(t *testing.T) {
	f, obj := blockerFixture(t)
	f.loops.criteriaMet = 0.5 // work done, not converged

	f.pass(t, obj.ID, reconcile.TriggerManual)
	for i := 2; i < noProgressPasses; i++ {
		f.again(t, obj.ID)
	}
	if got := f.state(t, obj.ID).Blockers; len(got) != 0 {
		t.Fatalf("blockers = %+v after %d passes, want none before %d", got, noProgressPasses-1, noProgressPasses)
	}
	f.again(t, obj.ID)
	wantBlockers(t, f.state(t, obj.ID), reconcile.Blocker{Kind: reconcile.BlockerNoProgress, Subject: string(obj.ID), Count: 1})
	if got := f.loops.count(); got != noProgressPasses {
		t.Errorf("the loop ran %d times, want %d: detection must not skip a pass", got, noProgressPasses)
	}

	// A new commit on the branch is progress.
	f.move("git", "bbb")
	f.again(t, obj.ID)
	if got := f.state(t, obj.ID).Blockers; len(got) != 0 {
		t.Errorf("blockers = %+v after a new commit, want none", got)
	}
}
