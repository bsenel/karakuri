package reconcile

// Phase 40 step 9, slice 2: the quota is the deployment's, not one stream's.
// While it is out no stream starts a pass; one probe by the highest-priority
// due stream, at the returned time or on a backoff, finds out whether it is
// back; the waiting streams then start in priority order.
//
// What the code does today, read before writing these (2026-11-17):
//   - reconcileNow (run.go) files a pass whose loop ended waiting_for_quota as
//     the deferral waiting_for_quota with DeferredUntil = end + RetryAfter
//     when the provider gave one (step 8). finish() moves no counter for a
//     Deferred outcome (ConsecutiveFailures stays) and floors that one
//     objective's next due time at DeferredUntil.
//   - Nothing else is told. Tick (service.go) hands every free slot to the due
//     objectives in priority order (slice 1), so every other stream still
//     starts passes into the same refusal and burns attempts.

import (
	"context"
	"slices"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// The three streams, highest priority first.
const (
	holdDelivery  objective.ObjectiveID = "delivery"
	holdUX        objective.ObjectiveID = "ux"
	holdDiscovery objective.ObjectiveID = "discovery"
)

// holdStreams declares the three streams on an hourly cadence. Only ux is due
// now, so the first tick runs ux alone; delivery and discovery come due half
// an hour later, while the quota is out.
func holdStreams(t *testing.T, f *fixture) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []struct {
		id       objective.ObjectiveID
		priority int
		due      time.Duration
	}{
		{holdDelivery, 30, 30 * time.Minute},
		{holdUX, 20, -time.Minute},
		{holdDiscovery, 10, 30 * time.Minute},
	} {
		f.declare(t, objective.Objective{
			ID:       s.id,
			Priority: s.priority,
			Cadence:  &objective.Cadence{Every: "1h"},
			Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
		})
		st := f.state(t, s.id)
		at := f.clock.Add(s.due)
		st.NextDueAt = &at
		st.NextReconcileAt = &at
		if err := f.store.SaveReconcileState(ctx, st); err != nil {
			t.Fatalf("seed due state: %v", err)
		}
	}
}

// backPass makes the fake loop end the way a run ends once the quota is back:
// completed, nothing refused.
func backPass(f *fixture) {
	f.loops.mu.Lock()
	defer f.loops.mu.Unlock()
	f.loops.paused = false
	f.loops.checkpoint = ""
	f.loops.status = objective.StatusCompleted
	f.loops.failure = ""
	f.loops.outcome = ""
	f.loops.retryAfter = 0
}

// idle reports whether no pass is running on this replica.
func (f *fixture) idle() bool {
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	return len(f.svc.running) == 0
}

// ranSince is the objectives the loop was run for after the first n runs.
func (f *fixture) ranSince(n int) []objective.ObjectiveID {
	f.loops.mu.Lock()
	defer f.loops.mu.Unlock()
	return append([]objective.ObjectiveID(nil), f.loops.ran[n:]...)
}

// tickAndSettle runs one Tick, waits for every pass it started to end, and
// returns the objectives the loop was run for during it.
func tickAndSettle(t *testing.T, f *fixture) []objective.ObjectiveID {
	t.Helper()
	before := f.loops.count()
	f.svc.Tick(context.Background())
	if !waitFor(5*time.Second, f.idle) {
		t.Fatal("the passes the tick started did not end")
	}
	return f.ranSince(before)
}

// sorted returns ids in a fixed order, for comparing passes that started
// together and so ran in no fixed order.
func sorted(ids []objective.ObjectiveID) []objective.ObjectiveID {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

// quotaOutAtStart runs ux into the quota refusal at the fixture's clock, with
// the provider's retry-after, and returns when that was.
func quotaOutAtStart(t *testing.T, f *fixture, retryAfter time.Duration) time.Time {
	t.Helper()
	holdStreams(t, f)
	t0 := f.clock
	quotaPass(f, retryAfter)
	if ran := tickAndSettle(t, f); !slices.Equal(ran, []objective.ObjectiveID{holdUX}) {
		t.Fatalf("the first tick ran %v, want only %q", ran, holdUX)
	}
	return t0
}

// (1) and (2): after one pass ends waiting_for_quota with a return time, no
// stream starts before that time, though two more come due; at that time
// exactly one pass starts, the highest-priority due stream's, and no other
// until it ends; (4) once it ends otherwise, the waiting streams start.
func TestNoStreamStartsWhileTheQuotaIsOutAndOneProbes(t *testing.T) {
	f := newFixture(t, Config{MaxConcurrent: 3, BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	ctx := context.Background()
	t0 := quotaOutAtStart(t, f, 2*time.Hour)

	// (1) Delivery and discovery are due, and there are slots for both.
	for _, at := range []time.Duration{time.Hour, 2*time.Hour - time.Second} {
		f.clock = t0.Add(at)
		if ran := tickAndSettle(t, f); len(ran) != 0 {
			t.Errorf("at +%v, before the quota returns, the tick started %v, want nothing", at, ran)
		}
	}

	// (2) At the return time: one probe, held open so a second tick can be
	// seen to start nothing beside it.
	f.clock = t0.Add(2 * time.Hour)
	backPass(f)
	gate := make(chan struct{})
	f.loops.mu.Lock()
	f.loops.gate = gate
	f.loops.mu.Unlock()
	before := f.loops.count()
	f.svc.Tick(ctx)
	if !waitFor(5*time.Second, func() bool { return f.loops.count() > before }) {
		t.Fatal("no probe started at the time the quota returns")
	}
	f.svc.Tick(ctx)
	time.Sleep(50 * time.Millisecond)
	probe := f.ranSince(before)
	f.loops.mu.Lock()
	f.loops.gate = nil
	f.loops.mu.Unlock()
	close(gate)
	if !waitFor(5*time.Second, f.idle) {
		t.Fatal("the probe did not end")
	}
	if !slices.Equal(probe, []objective.ObjectiveID{holdDelivery}) {
		t.Fatalf("while the probe ran, the passes started were %v, want only %q's: the highest-priority due stream probes alone", probe, holdDelivery)
	}

	// (4) The probe found the quota back: the waiting streams start.
	want := sorted([]objective.ObjectiveID{holdUX, holdDiscovery})
	if ran := tickAndSettle(t, f); !slices.Equal(sorted(ran), want) {
		t.Errorf("after the probe ended, the tick started %v, want %v", ran, want)
	}
}

// (4) With one slot, the waiting streams start one per tick, in priority
// order, after the probe.
func TestAfterTheProbeWaitingStreamsStartInPriorityOrder(t *testing.T) {
	f := newFixture(t, Config{MaxConcurrent: 1, BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	t0 := quotaOutAtStart(t, f, 2*time.Hour)

	f.clock = t0.Add(time.Hour)
	if ran := tickAndSettle(t, f); len(ran) != 0 {
		t.Errorf("before the quota returns, the tick started %v, want nothing", ran)
	}

	f.clock = t0.Add(2 * time.Hour)
	backPass(f)
	var got []objective.ObjectiveID
	for i := 0; i < 3; i++ {
		got = append(got, tickAndSettle(t, f)...)
	}
	want := []objective.ObjectiveID{holdDelivery, holdUX, holdDiscovery}
	if !slices.Equal(got, want) {
		t.Errorf("from the return time, the ticks started %v, want %v: the probe, then the waiting streams by priority", got, want)
	}
}

// (3) A probe that ends waiting_for_quota again leaves the quota out, and the
// next probe waits for the provider's time when it gave one, or for the
// declared backoff when it did not. A probe out of quota is a deferral like
// any other: its stream's ConsecutiveFailures does not move.
func TestAProbeStillOutOfQuotaWaitsBeforeTheNext(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryAfter time.Duration
		backoff    time.Duration
		wait       time.Duration
	}{
		{name: "provider's time", retryAfter: 3 * time.Hour, wait: 3 * time.Hour},
		// Longer than the hourly cadence, so delivery is due again before
		// the backoff ends and must still be held.
		{name: "declared backoff", backoff: 2 * time.Hour, wait: 2 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{MaxConcurrent: 3, BreakerFailures: 10, QuotaProbeBackoff: tc.backoff})
			f.use(t, map[string]string{"git": "aaa"})
			t0 := quotaOutAtStart(t, f, 2*time.Hour)

			// The probe at the return time is refused again.
			probeAt := t0.Add(2 * time.Hour)
			f.clock = probeAt
			quotaPass(f, tc.retryAfter)
			if ran := tickAndSettle(t, f); !slices.Equal(ran, []objective.ObjectiveID{holdDelivery}) {
				t.Fatalf("at the return time the tick started %v, want the one probe by %q", ran, holdDelivery)
			}
			st := f.state(t, holdDelivery)
			if st.ConsecutiveFailures != 0 || st.Paused {
				t.Errorf("a probe out of quota moved the breaker: consecutive_failures = %d, paused = %v", st.ConsecutiveFailures, st.Paused)
			}
			if h, err := f.svc.History(context.Background(), holdDelivery, 1); err != nil || len(h) != 1 || h[0].Deferred != waitingForQuota {
				t.Errorf("the probe's outcome = %+v (err %v), want deferred %q", h, err, waitingForQuota)
			}

			for _, at := range []time.Duration{tc.wait / 2, tc.wait - time.Minute} {
				f.clock = probeAt.Add(at)
				if ran := tickAndSettle(t, f); len(ran) != 0 {
					t.Errorf("at +%v after a probe still out of quota, the tick started %v, want nothing before +%v", at, ran, tc.wait)
				}
			}

			f.clock = probeAt.Add(tc.wait)
			backPass(f)
			if ran := tickAndSettle(t, f); !slices.Equal(ran, []objective.ObjectiveID{holdDelivery}) {
				t.Errorf("at +%v the tick started %v, want the one next probe by %q", tc.wait, ran, holdDelivery)
			}
		})
	}
}

// (5) A pass that ends with a plain failure, with no quota refusal, leaves the
// quota as it was: the other streams start when they are due, and the failure
// counts as it does today.
func TestAPlainFailureDoesNotHoldTheOtherStreams(t *testing.T) {
	f := newFixture(t, Config{MaxConcurrent: 3, BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	holdStreams(t, f)
	t0 := f.clock

	f.loops.mu.Lock()
	f.loops.status = objective.StatusFailed
	f.loops.failure = "the coding agent exited 1"
	f.loops.outcome = coreloop.RunErrored
	f.loops.mu.Unlock()
	if ran := tickAndSettle(t, f); !slices.Equal(ran, []objective.ObjectiveID{holdUX}) {
		t.Fatalf("the first tick ran %v, want only %q", ran, holdUX)
	}
	if st := f.state(t, holdUX); st.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d after a plain failure, want 1, as today", st.ConsecutiveFailures)
	}

	f.clock = t0.Add(30 * time.Minute)
	backPass(f)
	ran := tickAndSettle(t, f)
	for _, id := range []objective.ObjectiveID{holdDelivery, holdDiscovery} {
		if !slices.Contains(ran, id) {
			t.Errorf("when due after a plain failure, the tick started %v, want %q among them: a failure is not the quota", ran, id)
		}
	}
}
