package reconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 36 step 3. A loop the model provider refused twice ends failed, with
// the provider's rate limit as its error (step 1). The supervisor reads that
// text from the loop row and files the pass as a deferral, the way it files a
// spent budget: the objective did not misbehave, so the breaker and the stall
// detector do not count it, and the next pass is the cadence's to schedule.

// rateLimitText is what the loop writes for a run its provider refused: the
// loop's prefix around the core error, as reason.go builds it.
func rateLimitText() string {
	return fmt.Errorf("model provider rate limit: %w", &coreerrors.ProviderRateLimitError{
		Provider: "claude",
		Cause:    errors.New("API returned unexpected status code: 429: rate_limit_error"),
	}).Error()
}

func rateLimitFixture(t *testing.T, cfg Config) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, cfg)
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusFailed
	f.loops.failure = rateLimitText()
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// drainEvents returns what has been published so far, without waiting.
func drainEvents(ch <-chan event.Event) []event.Event {
	var out []event.Event
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestRateLimitedPassIsRecordedAsADeferral(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{})
	events, unsub := f.svc.hub.Subscribe(context.Background(), "obj:"+string(obj.ID))
	defer unsub()

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if got := f.loops.count(); got != 1 {
		t.Fatalf("the loop ran %d times, want 1", got)
	}
	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("recorded %d outcomes, want 1", len(history))
	}
	o := history[0]
	if o.Deferred != "provider_rate_limited" {
		t.Errorf("outcome.Deferred = %q, want provider_rate_limited", o.Deferred)
	}
	if o.Error != "" || o.Failed() {
		t.Errorf("outcome.Error = %q, want empty: a provider's rate limit is not the objective failing", o.Error)
	}
	if !o.DeferredUntil.IsZero() {
		t.Errorf("outcome.DeferredUntil = %v, want zero: no retry-after survives, the cadence decides", o.DeferredUntil)
	}
	if o.LoopID == "" {
		t.Error("the deferral lost the loop it came from; the loop row is where the provider's text is kept")
	}

	st := f.state(t, obj.ID)
	if st.ConsecutiveFailures != 0 || st.LastError != "" {
		t.Errorf("consecutive_failures = %d, last_error = %q; want 0 and empty", st.ConsecutiveFailures, st.LastError)
	}
	if st.Paused {
		t.Errorf("a rate-limited pass paused the objective: %s", st.PausedReason)
	}
	// Scheduled by the cadence alone: no failure backoff, no floor of its own.
	if st.NextDueAt == nil {
		t.Fatal("a rate-limited pass left the objective unscheduled")
	}
	if want := f.clock.Add(time.Hour); !st.NextDueAt.Equal(want) {
		t.Errorf("next due = %v, want %v from the 1h cadence", *st.NextDueAt, want)
	}

	// The reason, with the provider's name, is said where somebody watching
	// hears it: the deferred event.
	var deferred *event.Event
	for _, e := range drainEvents(events) {
		if e.Type == event.TypeReconcileDeferred {
			e := e
			deferred = &e
		}
		if e.Type == event.TypeReconcileFailed {
			t.Errorf("a rate-limited pass published %s: %v", e.Type, e.Payload)
		}
	}
	if deferred == nil {
		t.Fatalf("no %s event for a rate-limited pass", event.TypeReconcileDeferred)
	}
	if r, _ := deferred.Payload["reason"].(string); r != "provider_rate_limited" {
		t.Errorf("deferred event reason = %q, want provider_rate_limited", r)
	}
	detail, _ := deferred.Payload["detail"].(string)
	if !strings.Contains(detail, "claude") || !strings.Contains(detail, coreerrors.ErrProviderRateLimited.Error()) {
		t.Errorf("deferred event detail = %q, want the provider and the limit", detail)
	}
}

// The breaker is for an objective that misbehaves. A provider refusing on
// three passes in a row is not that, so it must not suspend the objective.
func TestRateLimitedPassesDoNotTripTheBreaker(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{BreakerFailures: 2})

	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.Paused {
		t.Fatalf("three rate-limited passes tripped the breaker: %s", st.PausedReason)
	}
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0", st.ConsecutiveFailures)
	}
	if pending, _ := f.store.ListPendingCheckpoints(context.Background(), ""); len(pending) != 0 {
		t.Errorf("%d checkpoints raised from rate limits, want none", len(pending))
	}
}

// Nor is a pass the provider refused a pass that got nowhere: the loop never
// planned, so it must not walk the objective into the stall detector.
func TestRateLimitedPassesDoNotCountAsAStall(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{StallReconciles: 2})

	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.Paused {
		t.Fatalf("three rate-limited passes were paused as stalled: %s", st.PausedReason)
	}
	if st.ScoreStreak != 0 {
		t.Errorf("score_streak = %d, want 0", st.ScoreStreak)
	}
}

// Any other failure behaves exactly as before step 3.
func TestOtherLoopFailuresAreStillFailures(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{})
	f.loops.failure = "claude: API returned unexpected status code: 500: overloaded"

	f.pass(t, obj.ID, reconcile.TriggerManual)

	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("recorded %d outcomes, want 1", len(history))
	}
	if history[0].Deferred != "" {
		t.Errorf("outcome.Deferred = %q for a non-rate-limit failure, want empty", history[0].Deferred)
	}
	if want := failureReason(coreloop.State{}); history[0].Error != want {
		t.Errorf("outcome.Error = %q, want today's %q", history[0].Error, want)
	}
	if st := f.state(t, obj.ID); st.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1", st.ConsecutiveFailures)
	}
}
