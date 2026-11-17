package reconcile

// Phase 40 step 7, the supervisor's and the digest's half: the endings the
// phase introduces are named outcomes, the circuit breaker does not count them,
// and the digest counts each under its own name, apart from failures.
//
// What the code does today, read before writing these (2026-11-17):
//   - reconcileNow files a loop that ended with status failed as a failure:
//     outcome.Error = failureReason(final), so finish() increments
//     ConsecutiveFailures. It looks at the row's Outcome only to keep an
//     errored run out of inconclusivePass. A loop refused by its bounds ends
//     with status failed (no criterion met), so today it is a failure.
//   - settleActiveLoop (step 4) frees a stream whose loop a restart ended and
//     re-runs the pass, but records no outcome for the pass that was cut off,
//     so nothing reaches the history the digest reads.
//   - The digest (internal/feature/report, summarise) reads reconcile outcomes
//     and counts Failed(), Escalated, Converged and drift. It has no count for
//     any of the endings below, and no name for them.
//
// Not written here, on purpose:
//   - Waiting for quota is step 8's ending, not delivered; its count belongs
//     with it.
//   - Phase 39's "pass ended" signal does not exist yet; no test asserts it.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/feature/report"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

const endingsTwin = "twin-endings"

// refusedPass makes the fake loop end the way the loop ends a run whose plan
// its bounds refused: completed, status failed because no criterion was met,
// no error text, and the outcome refused_by_bounds.
func refusedPass(f *fixture) {
	f.loops.paused = false
	f.loops.checkpoint = ""
	f.loops.status = objective.StatusFailed
	f.loops.criteriaMet = 0
	f.loops.failure = ""
	f.loops.outcome = coreloop.RunRefusedByBounds
}

// (a) A pass whose loop its bounds refused is not a failure: it does not move
// ConsecutiveFailures, does not trip the breaker, and is not filed as failed.
func TestARunRefusedByBoundsIsNotCountedAsAFailure(t *testing.T) {
	const breaker = 3
	f := newFixture(t, Config{BreakerFailures: breaker})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyActWithNotice, Ceiling: objective.AutonomyActWithNotice},
	})
	ctx := context.Background()

	// One short of the breaker, so a refusal counted as a failure trips it.
	st := f.state(t, obj.ID)
	st.ConsecutiveFailures = breaker - 1
	if err := f.store.SaveReconcileState(ctx, st); err != nil {
		t.Fatalf("seed consecutive failures: %v", err)
	}

	refusedPass(f)
	f.pass(t, obj.ID, reconcile.TriggerManual)

	after := f.state(t, obj.ID)
	if after.ConsecutiveFailures != breaker-1 {
		t.Errorf("consecutive_failures = %d after a pass refused by its bounds, want %d unchanged: a refusal is the bounds working, not the stream failing", after.ConsecutiveFailures, breaker-1)
	}
	if after.Paused {
		t.Errorf("the stream was paused (%s); a refusal must not trip the breaker", after.PausedReason)
	}
	history, err := f.svc.History(ctx, obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history holds %d outcomes, want the one refused pass", len(history))
	}
	if history[0].Failed() {
		t.Errorf("the refused pass is recorded as failed (%q)", history[0].Error)
	}
}

// (c) A digest over one errored pass, one pass a restart cut off and one pass
// its bounds refused reports one failure, not three, and counts each of the
// two new endings once under its own name.
//
// The passes go through the supervisor rather than being written by hand, so
// the test holds what reconcile records and what the digest reads to the same
// answer. "Under its own name" is read from the digest summary's JSON: a field
// or map key spelled as the outcome, holding the count. Where the count sits
// is the code's choice.
func TestTheDigestCountsTheNewEndingsApartFromFailures(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		TwinID:   endingsTwin,
		Title:    "deliver the roadmap",
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyActWithNotice, Ceiling: objective.AutonomyActWithNotice},
	})
	since := f.clock.Add(-time.Hour)

	// 1. Errored: the only failure among the three.
	f.loops.status = objective.StatusFailed
	f.loops.failure = "the coding agent exited 1"
	f.loops.outcome = coreloop.RunErrored
	f.pass(t, obj.ID, reconcile.TriggerManual)
	// Past the hourly cadence's floor, which holds back even a forced pass.
	f.clock = f.clock.Add(2 * time.Hour)

	// 2. Refused by its bounds.
	refusedPass(f)
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.clock = f.clock.Add(2 * time.Hour)

	// 3. Cut off by a restart: the pass's loop was approved and acting when
	// the process died, and the loop service ended it at boot (step 4). The
	// next pass runs it again, and that fresh pass converges.
	f.loops.status = objective.StatusCompleted
	f.loops.failure = ""
	f.loops.outcome = ""
	f.loops.paused = true
	f.loops.checkpoint = "cp-before-restart"
	f.pass(t, obj.ID, reconcile.TriggerManual)
	interrupted := f.state(t, obj.ID).ActiveLoopID
	if interrupted == "" {
		t.Fatal("the escalated pass left no active loop; the fixture does not reach the restart")
	}
	endByRestart(t, f, interrupted)
	f.clock = f.clock.Add(time.Minute)
	f.loops.paused = false
	f.loops.checkpoint = ""
	f.loops.criteriaMet = 1
	f.loops.outcome = coreloop.RunConverged
	f.pass(t, obj.ID, reconcile.TriggerManual)

	digests := report.NewService(f.store, nil, nil, karakuriquota.Deps{}, report.Config{})
	d, err := digests.Assemble(context.Background(), endingsTwin, since, f.clock.Add(time.Hour))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(d.Objectives) != 1 {
		t.Fatalf("the digest has %d objectives, want 1", len(d.Objectives))
	}
	sum := d.Objectives[0]
	if sum.Failures != 1 {
		t.Errorf("the digest reports %d failures, want 1: only the errored pass failed; a refusal and a restart are endings of their own", sum.Failures)
	}

	raw, err := json.Marshal(sum)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	for _, name := range []coreloop.RunOutcome{coreloop.RunInterruptedByRestart, coreloop.RunRefusedByBounds} {
		n, found := countUnder(tree, string(name))
		if !found {
			t.Errorf("the digest does not count %q under its own name; summary: %s", name, raw)
			continue
		}
		if n != 1 {
			t.Errorf("the digest counts %d %q passes, want 1", n, name)
		}
	}
}

// countUnder finds a key spelled name anywhere in a decoded JSON tree and
// returns the number it holds.
func countUnder(node any, name string) (int, bool) {
	switch v := node.(type) {
	case map[string]any:
		if n, ok := v[name].(float64); ok {
			return int(n), true
		}
		for _, child := range v {
			if n, ok := countUnder(child, name); ok {
				return n, true
			}
		}
	case []any:
		for _, child := range v {
			if n, ok := countUnder(child, name); ok {
				return n, true
			}
		}
	}
	return 0, false
}
