package report

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 step 6: the digest lines for criterion states.
//
// The per-pass states these tests read are already stored: a reconcile
// outcome with a LoopID points at that loop's row, and coreloop.State.Criteria
// holds what its last verify step settled. Loop rows are never deleted, so the
// assembler can walk a window's passes newest first without anything new being
// written as work happens. Sense-only passes (no LoopID) settle nothing and do
// not break a run of passes.

// A required criterion that stayed inconclusive for two passes in a row is
// listed with its reason. Otherwise a stream that can never be measured would
// neither complete nor trip the breaker, and nobody would hear of it.
func TestDigestListsACriterionInconclusiveForTwoPasses(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	obj := seedWithCriteria(t, store, "obj-1", "keep the build green",
		required("ci-green", "CI is green on the stream's PR"),
		required("pr-open", "an open PR with tests"),
		required("tests", "the unit tests pass"),
		required("review", "a reviewer approved"),
	)

	since := clock.Add(-24 * time.Hour)
	seedPass(t, store, obj.ID, "p1", since.Add(time.Hour),
		settled("ci-green", objective.CriterionInconclusive, objective.ReasonNotRun),
		settled("pr-open", objective.CriterionInconclusive, objective.ReasonNoData),
		settled("tests", objective.CriterionMet, ""),
		settled("review", objective.CriterionMet, ""),
	)
	// Cheap passes in between settle nothing and must not break the run.
	for i := 0; i < 3; i++ {
		mustOutcome(t, store, reconcile.Outcome{
			ID: "s" + id(i), ObjectiveID: obj.ID, Trigger: reconcile.TriggerSchedule,
			StartedAt: since.Add(2*time.Hour + time.Duration(i)*time.Minute),
		})
	}
	seedPass(t, store, obj.ID, "p2", since.Add(3*time.Hour),
		settled("ci-green", objective.CriterionInconclusive, objective.ReasonNotRun),
		settled("pr-open", objective.CriterionMet, ""),
		settled("tests", objective.CriterionNotMet, ""),
		settled("review", objective.CriterionInconclusive, objective.ReasonNoData),
	)

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	sum := onlyObjective(t, d)

	ci, ok := lineFor(sum, "ci-green")
	if !ok {
		t.Fatalf("ci-green, inconclusive for two passes, is not listed: %+v", sum.Criteria)
	}
	if ci.State != objective.CriterionInconclusive || ci.Reason != objective.ReasonNotRun {
		t.Errorf("ci-green = %s: %s, want inconclusive: not_run", ci.State, ci.Reason)
	}
	if ci.Passes != 2 {
		t.Errorf("ci-green passes = %d, want 2", ci.Passes)
	}
	if ci.Description == "" {
		t.Error("ci-green carries no description; the reader should not have to look the ID up")
	}

	// not_met in the latest pass is listed after one pass.
	if tests, ok := lineFor(sum, "tests"); !ok {
		t.Error("tests, not_met in the latest pass, is not listed")
	} else if tests.State != objective.CriterionNotMet || tests.Passes != 1 {
		t.Errorf("tests = %s for %d passes, want not_met for 1", tests.State, tests.Passes)
	}
	// Met now, whatever it was before.
	if _, ok := lineFor(sum, "pr-open"); ok {
		t.Error("pr-open is met in the latest pass and was listed")
	}
	// Inconclusive for one pass only is not yet news.
	if _, ok := lineFor(sum, "review"); ok {
		t.Error("review was inconclusive for one pass only and was listed")
	}
	if sum.Progress != nil {
		t.Errorf("progress = %+v for an objective with no optional criterion, want nil", sum.Progress)
	}

	out := Plain(d)
	line := renderedLine(out, "ci-green")
	if line == "" {
		t.Fatalf("the rendered digest does not name ci-green:\n%s", out)
	}
	if !strings.Contains(line, "inconclusive: not_run") || !strings.Contains(line, "2 passes") {
		t.Errorf("ci-green renders as %q, want its state, reason and \"2 passes\"", line)
	}
}

// A required criterion its verifier never settled is inconclusive: not_run in
// the digest, never met and never left blank. Gherkin's undefined step that
// became a pass is the failure this guards against.
func TestDigestShowsACriterionWhoseVerifierNeverRanAsNotRun(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	obj := seedWithCriteria(t, store, "obj-1", "keep the build green",
		required("ci-green", "CI is green on the stream's PR"),
	)

	// Both passes carry the criterion with no state: its verifier never ran.
	since := clock.Add(-24 * time.Hour)
	seedPass(t, store, obj.ID, "p1", since.Add(time.Hour), settled("ci-green", "", ""))
	seedPass(t, store, obj.ID, "p2", since.Add(2*time.Hour), settled("ci-green", "", ""))

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	sum := onlyObjective(t, d)

	ci, ok := lineFor(sum, "ci-green")
	if !ok {
		t.Fatalf("a criterion whose verifier never ran is not listed: %+v", sum.Criteria)
	}
	if ci.State != objective.CriterionInconclusive || ci.Reason != objective.ReasonNotRun {
		t.Errorf("ci-green = %q: %q, want inconclusive: not_run", ci.State, ci.Reason)
	}
	if ci.Passes != 2 {
		t.Errorf("ci-green passes = %d, want 2", ci.Passes)
	}

	line := renderedLine(Plain(d), "ci-green")
	if !strings.Contains(line, "inconclusive: not_run") {
		t.Errorf("ci-green renders as %q, want \"inconclusive: not_run\"", line)
	}
	if strings.Contains(line, " met") && !strings.Contains(line, "not_met") {
		t.Errorf("ci-green renders as %q, which reads as met", line)
	}
}

// An optional weighted criterion is progress, apart from the gate: 0.7 of 1.0
// earned while every required criterion is met is an expected partial score,
// and must not read as a failure.
func TestDigestShowsOptionalWeightedCriteriaAsProgressApartFromTheGate(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	docs := optional("docs", "the docs mention the new flag", 0.7)
	perf := optional("perf", "startup under 200 ms", 0.3)
	obj := seedWithCriteria(t, store, "obj-1", "ship the flag",
		required("ci-green", "CI is green on the stream's PR"), docs, perf,
	)

	since := clock.Add(-24 * time.Hour)
	for i, at := range []time.Time{since.Add(time.Hour), since.Add(2 * time.Hour)} {
		seedPass(t, store, obj.ID, "p"+id(i), at,
			settled("ci-green", objective.CriterionMet, ""),
			withState(docs, objective.CriterionMet, ""),
			withState(perf, objective.CriterionNotMet, ""),
		)
	}

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	sum := onlyObjective(t, d)

	if sum.Progress == nil {
		t.Fatal("no progress for an objective with optional weighted criteria")
	}
	if math.Abs(sum.Progress.Earned-0.7) > 1e-9 || math.Abs(sum.Progress.Of-1.0) > 1e-9 {
		t.Errorf("progress = %v of %v, want 0.7 of 1.0", sum.Progress.Earned, sum.Progress.Of)
	}
	// The gate lists required criteria only; perf's not_met is progress, not
	// a gate line.
	for _, c := range sum.Criteria {
		if c.ID == "docs" || c.ID == "perf" {
			t.Errorf("optional criterion %q is listed with the gate: %+v", c.ID, c)
		}
	}

	out := Plain(d)
	if !strings.Contains(out, "0.7 of 1.0") {
		t.Errorf("the rendered digest does not show progress as \"0.7 of 1.0\":\n%s", out)
	}
	if strings.Contains(out, "not_met") {
		t.Errorf("an optional criterion's partial score renders as not_met:\n%s", out)
	}
}

func seedWithCriteria(t *testing.T, store storage.StorageAdapter, objID, title string, criteria ...objective.Criterion) objective.Objective {
	t.Helper()
	obj := seedStanding(t, store, "twin-1", objID, title)
	obj.SuccessCriteria = criteria
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return obj
}

// seedPass records one expensive pass: its outcome, and the loop row holding
// the criterion states its verify step settled.
func seedPass(t *testing.T, store storage.StorageAdapter, objID objective.ObjectiveID, passID string, at time.Time, criteria ...objective.Criterion) {
	t.Helper()
	loopID := "loop-" + passID
	if err := store.SaveLoopState(context.Background(), coreloop.State{
		LoopID: loopID, ObjectiveID: objID, TwinID: "twin-1",
		Completed: true, Criteria: criteria,
		CreatedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
	mustOutcome(t, store, reconcile.Outcome{
		ID: passID, ObjectiveID: objID, LoopID: loopID,
		Trigger: reconcile.TriggerSchedule, StartedAt: at, EndedAt: at,
	})
}

func required(criterionID, description string) objective.Criterion {
	return objective.Criterion{ID: criterionID, Description: description, Weight: 1}
}

func optional(criterionID, description string, weight float64) objective.Criterion {
	no := false
	return objective.Criterion{ID: criterionID, Description: description, Weight: weight, Required: &no}
}

// settled is a required criterion as one pass left it.
func settled(criterionID string, state objective.CriterionState, reason objective.InconclusiveReason) objective.Criterion {
	return withState(required(criterionID, criterionID+" holds"), state, reason)
}

func withState(c objective.Criterion, state objective.CriterionState, reason objective.InconclusiveReason) objective.Criterion {
	c.State, c.Reason = state, reason
	return c
}

func onlyObjective(t *testing.T, d digest.Digest) digest.ObjectiveSummary {
	t.Helper()
	if len(d.Objectives) != 1 {
		t.Fatalf("got %d objectives, want 1", len(d.Objectives))
	}
	return d.Objectives[0]
}

func lineFor(sum digest.ObjectiveSummary, criterionID string) (digest.CriterionLine, bool) {
	for _, c := range sum.Criteria {
		if c.ID == criterionID {
			return c, true
		}
	}
	return digest.CriterionLine{}, false
}

// renderedLine is the first line of the plain digest naming the criterion.
func renderedLine(out, criterionID string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, criterionID) {
			return l
		}
	}
	return ""
}
