package reconcile

// Phase 40 step 14b, reconcile's half: the request of a stream's next ordinary
// pass carries the one blocker that pass is to diagnose and fix, with the
// remedy its template declares for that kind. Quota is waiting (steps 8 and 9)
// and is never carried. The loop is taught nothing (AGENTS.md rule 8): these
// tests read coreloop.Request.Unblock off the request the real pass path built,
// the way resume_brief_test.go reads Request.Resume.

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/domain"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

var t0 = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

func blk(kind reconcile.BlockerKind, subject string, count int, first time.Time) reconcile.Blocker {
	return reconcile.Blocker{Kind: kind, Subject: subject, Count: count, FirstSeen: first, LastSeen: first.Add(time.Hour)}
}

// unblockRequest stores the blockers on the stream's state, runs its next pass
// and returns the request that pass gave the loop.
func unblockRequest(t *testing.T, f *fixture, obj objective.Objective, blockers ...reconcile.Blocker) coreloop.Request {
	t.Helper()
	log := &requestLog{fakeLoop: f.loops}
	f.svc.loops = log
	st := f.state(t, obj.ID)
	st.Blockers = blockers
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	f.pass(t, obj.ID, reconcile.TriggerManual)
	return log.last(t)
}

func plainStream(t *testing.T) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// (a) One verify_failed blocker reaches the request with its kind, subject and
// count; a pack that declared no remedy for the kind sends an empty one and the
// blocker is still carried.
func TestTheNextPassIsGivenTheStreamsBlocker(t *testing.T) {
	f, obj := plainStream(t)
	first := t0
	req := unblockRequest(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 2, first))

	got := req.Unblock
	if got == nil {
		t.Fatal("request.Unblock is nil, want the verify_failed blocker")
	}
	if got.Kind != "verify_failed" || got.Subject != "tests-pass" || got.Count != 2 {
		t.Errorf("Unblock = %+v, want verify_failed on tests-pass seen 2 times", *got)
	}
	if !got.FirstSeen.Equal(first) {
		t.Errorf("Unblock.FirstSeen = %v, want %v", got.FirstSeen, first)
	}
	if got.Remedy != "" {
		t.Errorf("Unblock.Remedy = %q, want empty: no pack declared one for this kind", got.Remedy)
	}
}

func TestNoBlockerOrOnlyQuotaGivesThePassNoBlocker(t *testing.T) {
	for name, blockers := range map[string][]reconcile.Blocker{
		"none":       nil,
		"only quota": {blk(reconcile.BlockerDeferredQuota, "obj-1", 4, t0)},
	} {
		t.Run(name, func(t *testing.T) {
			f, obj := plainStream(t)
			if got := unblockRequest(t, f, obj, blockers...).Unblock; got != nil {
				t.Fatalf("request.Unblock = %+v, want nil: no diagnose-and-fix pass is spent on it", *got)
			}
		})
	}
}

// Quota never crowds out a blocker that can be fixed.
func TestQuotaIsSkippedWhenAnotherBlockerIsThere(t *testing.T) {
	f, obj := plainStream(t)
	req := unblockRequest(t, f, obj,
		blk(reconcile.BlockerDeferredQuota, "obj-1", 9, t0),
		blk(reconcile.BlockerPlanError, "obj-1", 1, t0.Add(time.Hour)))
	if req.Unblock == nil || req.Unblock.Kind != "plan_error" {
		t.Fatalf("request.Unblock = %+v, want the plan_error blocker", req.Unblock)
	}
}

// With two blockers that are not quota the choice is the same on every pass:
// highest count, then oldest first seen, then kind, then subject.
func TestTheBlockerChosenIsDeterministic(t *testing.T) {
	older, newer := t0, t0.Add(2*time.Hour)
	for _, tc := range []struct {
		name       string
		blockers   []reconcile.Blocker
		kind, subj string
	}{
		{"highest count", []reconcile.Blocker{
			blk(reconcile.BlockerVerifyFailed, "a", 2, older),
			blk(reconcile.BlockerPlanError, "obj-1", 5, newer)}, "plan_error", "obj-1"},
		{"oldest first seen on a tie", []reconcile.Blocker{
			blk(reconcile.BlockerPlanError, "obj-1", 3, newer),
			blk(reconcile.BlockerVerifyFailed, "a", 3, older)}, "verify_failed", "a"},
		{"kind on a tie of both", []reconcile.Blocker{
			blk(reconcile.BlockerVerifyFailed, "a", 3, older),
			blk(reconcile.BlockerPlanError, "z", 3, older)}, "plan_error", "z"},
		{"subject last", []reconcile.Blocker{
			blk(reconcile.BlockerVerifyFailed, "b", 3, older),
			blk(reconcile.BlockerVerifyFailed, "a", 3, older)}, "verify_failed", "a"},
	} {
		for _, reversed := range []bool{false, true} {
			bs := append([]reconcile.Blocker(nil), tc.blockers...)
			if reversed {
				bs[0], bs[1] = bs[1], bs[0]
			}
			f, obj := plainStream(t)
			got := unblockRequest(t, f, obj, bs...).Unblock
			if got == nil || got.Kind != tc.kind || got.Subject != tc.subj {
				t.Errorf("%s (reversed=%v): Unblock = %+v, want %s on %s", tc.name, reversed, got, tc.kind, tc.subj)
			}
		}
	}
}

// (b) The pack's remedy for the blocker's kind reaches the request.
func TestTheTemplatesRemedyForTheBlockersKindReachesTheRequest(t *testing.T) {
	for _, tmpl := range software.New().ObjectiveTemplates() {
		switch tmpl.ID {
		case "software.objective.market_discovery", "software.objective.engineering_backlog",
			"software.objective.ux_improvement", "software.objective.roadmap_delivery":
		default:
			continue
		}
		t.Run(tmpl.ID, func(t *testing.T) {
			want := tmpl.BlockerRemedies[string(reconcile.BlockerUnmergeablePR)]
			if want == "" {
				t.Fatalf("%s declares no unmergeable_pr remedy", tmpl.ID)
			}
			f := newFixture(t, Config{BreakerFailures: 10})
			reg := domain.NewRegistry()
			if err := reg.Register(context.Background(), software.New(), domain.Config{}); err != nil {
				t.Fatalf("register software: %v", err)
			}
			f.svc.domReg = reg
			f.use(t, map[string]string{"git": "aaa"})
			obj := f.declare(t, objective.Objective{
				AdditionalDomains: []string{"software"},
				TemplateID:        tmpl.ID,
				Cadence:           &objective.Cadence{Every: "1h"},
				Autonomy:          &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
			})

			got := unblockRequest(t, f, obj, blk(reconcile.BlockerUnmergeablePR, "karakuri/phase-40-x", 1, t0)).Unblock
			if got == nil || got.Kind != "unmergeable_pr" {
				t.Fatalf("request.Unblock = %+v, want the unmergeable_pr blocker", got)
			}
			if got.Remedy != want {
				t.Errorf("Unblock.Remedy = %q, want the template's %q", got.Remedy, want)
			}
		})
	}
}
