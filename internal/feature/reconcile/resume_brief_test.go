package reconcile

// Phase 40 step 9a, reconcile's half: the pass that follows a deferral for
// quota (step 8) or an interrupted_by_restart ending (step 4) is given, in its
// request, the commits already on the stream's branch and the success
// criteria the cut-off pass had not settled. After any other ending it is
// given neither.
//
// What the code does today, read before writing these (2026-11-21):
//   - reconcileNow (run.go) builds coreloop.Request from the objective, twin,
//     agent and rung alone; nothing of the previous pass reaches it.
//   - A waiting_for_quota pass keeps no ActiveLoopID; its loop is found
//     through the last outcome's LoopID. A restart's loop is the supervisor's
//     ActiveLoopID until settleActiveLoop records it and re-runs the pass.
//   - The loop row keeps the criteria the last verify step settled
//     (coreloop.State.Criteria, Phase 38), each with a State.
//   - Where the branch is, and its commits ahead of its base, is read by
//     Service.branchCommits (a seam; production wires the version-control
//     adapter's CompareCommits, as domains/software/branches_without_pr.go
//     does).
//
// The fake loop is wrapped, not edited, to keep each request.

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// requestLog keeps every request the loop was started with.
type requestLog struct {
	*fakeLoop
	mu   sync.Mutex
	reqs []coreloop.Request
}

func (r *requestLog) Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
	return r.fakeLoop.Run(ctx, req)
}

func (r *requestLog) last(t *testing.T) coreloop.Request {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		t.Fatal("the loop was never started")
	}
	return r.reqs[len(r.reqs)-1]
}

func resumeCriteria() []objective.Criterion {
	return []objective.Criterion{
		{ID: "pr", Description: "pull request", Weight: 0.25, State: objective.CriterionMet},
		{ID: "ci", Description: "checks green", Weight: 0.25, State: objective.CriterionNotMet},
		{ID: "tests-pass", Description: "tests pass", Weight: 0.25, State: objective.CriterionInconclusive},
		{ID: "docs", Description: "docs updated", Weight: 0.25, State: objective.CriterionMet},
	}
}

var (
	resumeCommits   = []string{"aaa111 feat: first action", "bbb222 feat: second action"}
	resumeUnsettled = []string{"ci", "tests-pass"}
)

// resumeFixture declares a standing stream whose branch reads as given, and
// records the requests its loop receives.
type resumeFixture struct {
	*fixture
	log *requestLog
	obj objective.Objective
}

func newResumeFixture(t *testing.T, commits []string, readErr error) resumeFixture {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	log := &requestLog{fakeLoop: f.loops}
	f.svc.loops = log
	f.svc.branchCommits = func(context.Context, objective.Objective) (string, []string, error) {
		if readErr != nil {
			return "karakuri/phase-40-step-09-quota-order", nil, readErr
		}
		return "karakuri/phase-40-step-09-quota-order", commits, nil
	}
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return resumeFixture{fixture: f, log: log, obj: obj}
}

// firstPass ends the way each case says and leaves the stream ready for the
// next pass, past the cadence's floor.
type firstPass func(t *testing.T, r resumeFixture)

func endsWaitingForQuota(t *testing.T, r resumeFixture) {
	r.loops.criteria = resumeCriteria()
	quotaPass(r.fixture, 2*time.Hour)
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	r.clock = r.clock.Add(3 * time.Hour)
}

func endsInterruptedByRestart(t *testing.T, r resumeFixture) {
	// The pass escalated, its loop was approved and was acting when the
	// process died.
	r.loops.criteria = resumeCriteria()
	r.loops.paused = true
	r.loops.checkpoint = "cp-before-restart"
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	active := r.state(t, r.obj.ID).ActiveLoopID
	if active == "" {
		t.Fatal("the escalated pass left no active loop; the fixture does not reach the case")
	}
	endByRestart(t, r.fixture, active)
	r.loops.paused = false
	r.loops.checkpoint = ""
	r.loops.outcome = ""
}

func endsInSuccess(t *testing.T, r resumeFixture) {
	r.loops.criteria = resumeCriteria()
	r.loops.criteriaMet = 1
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	r.clock = r.clock.Add(3 * time.Hour)
}

func endsInFailure(t *testing.T, r resumeFixture) {
	r.loops.criteria = resumeCriteria()
	r.loops.status = objective.StatusFailed
	r.loops.failure = "the coding agent exited 1"
	r.loops.outcome = coreloop.RunErrored
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	r.clock = r.clock.Add(3 * time.Hour)
	r.loops.status = objective.StatusCompleted
	r.loops.failure = ""
	r.loops.outcome = ""
}

// (a) After a cut-off ending the next request carries the branch's commits and
// the criteria not settled as met; after any other ending it carries neither.
func TestThePassAfterACutOffPassIsGivenItsBranchAndWhatIsLeft(t *testing.T) {
	for _, tc := range []struct {
		name    string
		first   firstPass
		resumed bool
	}{
		{"after waiting_for_quota", endsWaitingForQuota, true},
		{"after interrupted_by_restart", endsInterruptedByRestart, true},
		{"after an ordinary success", endsInSuccess, false},
		{"after an ordinary failure", endsInFailure, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newResumeFixture(t, resumeCommits, nil)
			tc.first(t, r)
			before := r.loops.count()

			r.pass(t, r.obj.ID, reconcile.TriggerManual)

			if r.loops.count() != before+1 {
				t.Fatalf("the loop ran %d times after the first pass, want 1 more", r.loops.count()-before)
			}
			got := r.log.last(t).Resume
			if !tc.resumed {
				if got != nil {
					t.Fatalf("request.Resume = %+v, want nil: the previous pass was not cut off", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("request.Resume is nil, want the branch's commits and the unsettled criteria")
			}
			if got.Branch != "karakuri/phase-40-step-09-quota-order" {
				t.Errorf("Resume.Branch = %q, want the stream's branch", got.Branch)
			}
			if !reflect.DeepEqual(got.Commits, resumeCommits) {
				t.Errorf("Resume.Commits = %v, want %v", got.Commits, resumeCommits)
			}
			if got.CommitsUnreadable != "" {
				t.Errorf("Resume.CommitsUnreadable = %q, want empty: they were read", got.CommitsUnreadable)
			}
			if !reflect.DeepEqual(got.Unsettled, resumeUnsettled) {
				t.Errorf("Resume.Unsettled = %v, want %v: the criteria not settled as met", got.Unsettled, resumeUnsettled)
			}
		})
	}
}

// (b) A branch read that fails is said so; it is never an empty list that
// reads as "nothing is on the branch" (AGENTS.md rule 10). A branch that was
// read and holds nothing is the opposite case and says none.
func TestAnUnreadableBranchIsSaidSoAndNeverAnEmptyList(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first firstPass
	}{
		{"after waiting_for_quota", endsWaitingForQuota},
		{"after interrupted_by_restart", endsInterruptedByRestart},
	} {
		t.Run(tc.name+", read fails", func(t *testing.T) {
			r := newResumeFixture(t, nil, errors.New("compare: 403 rate limit exceeded"))
			tc.first(t, r)

			r.pass(t, r.obj.ID, reconcile.TriggerManual)

			got := r.log.last(t).Resume
			if got == nil {
				t.Fatal("request.Resume is nil: the pass was cut off, and an unreadable branch must still be said")
			}
			if got.CommitsUnreadable == "" {
				t.Error("Resume.CommitsUnreadable is empty: an unreadable branch must say it could not be read")
			}
			if len(got.Commits) != 0 {
				t.Errorf("Resume.Commits = %v, want none listed when they could not be read", got.Commits)
			}
			// What was left does not depend on the branch.
			if !reflect.DeepEqual(got.Unsettled, resumeUnsettled) {
				t.Errorf("Resume.Unsettled = %v, want %v", got.Unsettled, resumeUnsettled)
			}
		})
		t.Run(tc.name+", read and empty", func(t *testing.T) {
			r := newResumeFixture(t, []string{}, nil)
			tc.first(t, r)

			r.pass(t, r.obj.ID, reconcile.TriggerManual)

			got := r.log.last(t).Resume
			if got == nil {
				t.Fatal("request.Resume is nil, want the brief")
			}
			if got.CommitsUnreadable != "" {
				t.Errorf("Resume.CommitsUnreadable = %q, want empty: the branch was read", got.CommitsUnreadable)
			}
			if len(got.Commits) != 0 {
				t.Errorf("Resume.Commits = %v, want none", got.Commits)
			}
		})
	}
}

// (c) The clock side of the acceptance: no pass runs before the quota's return
// time; the first one after it is given the brief.
func TestNoPassRunsBeforeTheQuotaReturnsAndTheNextOneIsGivenTheBrief(t *testing.T) {
	r := newResumeFixture(t, resumeCommits, nil)
	r.loops.criteria = resumeCriteria()
	quotaPass(r.fixture, 2*time.Hour)
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	ran := r.loops.count()

	r.clock = r.clock.Add(time.Hour) // inside the two hours the provider asked for
	tickAndSettle(t, r.fixture)
	if r.loops.count() != ran {
		t.Fatalf("a pass ran an hour before the quota returns")
	}

	r.clock = r.clock.Add(time.Hour + time.Minute)
	tickAndSettle(t, r.fixture)
	if r.loops.count() != ran+1 {
		t.Fatalf("no pass ran after the quota returned (loop runs %d, want %d)", r.loops.count(), ran+1)
	}
	got := r.log.last(t).Resume
	if got == nil || !reflect.DeepEqual(got.Commits, resumeCommits) || !reflect.DeepEqual(got.Unsettled, resumeUnsettled) {
		t.Fatalf("request.Resume = %+v, want the two commits and the unsettled criteria", got)
	}
}
