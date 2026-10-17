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
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// rateLimitText is what a loop the provider refused twice ends on: the runner
// finalizes it failed with the *ProviderRateLimitError the reason step gave up
// on.
var rateLimitText = (&coreerrors.ProviderRateLimitError{
	Provider: "claude",
	Cause:    errors.New("status code: 429"),
}).Error()

// rateLimitFixture is an objective allowed to act, over a loop that ends
// failed on the given error.
func rateLimitFixture(t *testing.T, cfg Config, loopErr string) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, cfg)
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusFailed
	f.loops.loopError = loopErr
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// A provider that refused is a deferral, for the reason a spent budget is:
// nothing in the deployment misbehaved, the conditions for spending were
// simply not met. Counting it as a failure would walk a healthy objective into
// the breaker for a quota somebody else enforces.
func TestRateLimitedPassIsDeferredNotFailed(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{}, rateLimitText)

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if got := f.loops.count(); got != 1 {
		t.Fatalf("ran %d loops, want 1", got)
	}
	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("a rate-limited pass recorded no outcome; the history cannot say why nothing happened")
	}
	o := history[0]
	if o.Deferred != "provider_rate_limited" {
		t.Errorf("outcome.Deferred = %q, want provider_rate_limited", o.Deferred)
	}
	if o.Error != "" {
		t.Errorf("outcome.Error = %q, want empty; a provider refusing is not this deployment failing", o.Error)
	}
	if o.Failed() {
		t.Error("a rate-limited pass reported itself as a failed pass")
	}
	st := f.state(t, obj.ID)
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after a rate-limited pass, want 0", st.ConsecutiveFailures)
	}
}

// Two refusals in a row are still not two failures, so a breaker set to trip
// at two leaves the objective in the rotation.
func TestRateLimitedPassesDoNotTripTheBreaker(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{BreakerFailures: 2}, rateLimitText)

	for i := 0; i < 2; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.Paused {
		t.Fatalf("two rate-limited passes paused the objective: %q", st.PausedReason)
	}
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after two rate-limited passes, want 0", st.ConsecutiveFailures)
	}
	pending, _ := f.store.ListPendingCheckpoints(context.Background(), "")
	for _, cp := range pending {
		if cp.Reason == "reconcile_circuit_breaker" {
			t.Errorf("the breaker asked a human about a provider's quota: %q", cp.Summary)
		}
	}
}

// Any other failure is still a failure: it is recorded, counted, and trips the
// breaker as before. The deferral is for a provider refusing, not for a loop
// ending failed.
func TestOtherLoopFailureStillTripsTheBreaker(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{BreakerFailures: 2}, "boom")

	f.pass(t, obj.ID, reconcile.TriggerManual)

	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("a failed pass recorded no outcome")
	}
	if history[0].Error == "" {
		t.Error("a loop that failed on 'boom' recorded no error")
	}
	if history[0].Deferred != "" {
		t.Errorf("a loop that failed on 'boom' was deferred as %q", history[0].Deferred)
	}
	if st := f.state(t, obj.ID); st.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d after one failed pass, want 1", st.ConsecutiveFailures)
	}

	f.clock = f.clock.Add(time.Hour)
	f.pass(t, obj.ID, reconcile.TriggerManual)

	if st := f.state(t, obj.ID); !st.Paused {
		t.Fatalf("still running after 2 consecutive failures (count=%d)", st.ConsecutiveFailures)
	}
}

// The outcome carries a reason code and no free text, so the provider is named
// where the budget deferral puts its detail: on the reconcile_deferred event.
// An operator reading the stream should learn which provider refused without
// opening the loop's audit log.
func TestRateLimitedDeferralNamesTheProvider(t *testing.T) {
	f, obj := rateLimitFixture(t, Config{}, rateLimitText)
	events, unsub := f.svc.hub.Subscribe(context.Background(), "obj:"+string(obj.ID))
	defer unsub()

	f.pass(t, obj.ID, reconcile.TriggerManual)

	var deferred []event.Event
	for drained := false; !drained; {
		select {
		case e := <-events:
			if e.Type == event.TypeReconcileDeferred {
				deferred = append(deferred, e)
			}
		default:
			drained = true
		}
	}
	if len(deferred) != 1 {
		t.Fatalf("published %d reconcile_deferred events, want 1", len(deferred))
	}
	payload := deferred[0].Payload
	if payload["reason"] != "provider_rate_limited" {
		t.Errorf("deferred reason = %v, want provider_rate_limited", payload["reason"])
	}
	named := false
	for _, v := range payload {
		if strings.Contains(fmt.Sprint(v), "claude") {
			named = true
		}
	}
	if !named {
		t.Errorf("the deferral does not name the provider that refused; payload = %v", payload)
	}
}
