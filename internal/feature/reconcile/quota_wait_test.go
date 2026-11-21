package reconcile

// Phase 40 step 8, the supervisor's and the digest's half: a pass whose loop
// ended waiting for quota is a deferral, until the quota returns where the
// provider said when, and never a failure.
//
// What the code does today, read before writing these (2026-11-17):
//   - reconcileNow (run.go) files a loop row with status failed whose error
//     text holds coreerrors.ErrProviderRateLimited as the deferral
//     provider_rate_limited (Phase 36), with no DeferredUntil, because the
//     planner's rate limit keeps no retry-after. It reads the row's Outcome
//     only for refused_by_bounds (step 7). A coding agent's quota refusal
//     carries the same error type, so a loop that stored its text would be
//     filed as provider_rate_limited, not waiting_for_quota, and one that
//     stored none would be filed as a failure.
//   - The budget_exhausted deferral (run.go, pass) is the treatment step 8
//     asks for: an outcome with Deferred and DeferredUntil, a
//     TypeReconcileDeferred event with the reason, and finish() moving no
//     counter for a Deferred outcome and flooring the next due time at
//     DeferredUntil.
//   - The digest (internal/feature/report, summarise) counts Failed() passes,
//     and Ending names under Endings; it counts no Deferred reason except
//     budget_exhausted, in its own section.
//   - The loop row's RetryAfter is this step's scaffolding; storage does not
//     persist it yet, so it reads back zero.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/feature/report"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

const waitingForQuota = string(coreloop.RunWaitingForQuota)

// quotaPass makes the fake loop end the way a run whose coding agent was
// refused for quota ends: completed, status failed because no criterion was
// met, the outcome waiting_for_quota and the provider's retry-after. The row
// keeps the refusal's text as well, which today's provider_rate_limited match
// reads: the named outcome must decide, not the text.
func quotaPass(f *fixture, retryAfter time.Duration) {
	f.loops.paused = false
	f.loops.checkpoint = ""
	f.loops.status = objective.StatusFailed
	f.loops.criteriaMet = 0
	f.loops.failure = (&coreerrors.ProviderRateLimitError{
		Provider:   "claude_code",
		RetryAfter: retryAfter,
		Cause:      errors.New("claude_code: exit 1 (stderr: ); last output: You've hit your session limit"),
	}).Error()
	f.loops.outcome = coreloop.RunWaitingForQuota
	f.loops.retryAfter = retryAfter
}

// (b) One pass out of quota records one deferral named waiting_for_quota,
// until now plus the retry-after when the provider gave one and with no time
// when it did not, says so on TypeReconcileDeferred, leaves
// ConsecutiveFailures where it was and does not trip the breaker.
func TestAPassOutOfQuotaIsDeferredUntilTheQuotaReturns(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryAfter time.Duration
	}{
		{"with a retry-after", 2 * time.Hour},
		{"without one", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const breaker = 3
			f := newFixture(t, Config{BreakerFailures: breaker})
			f.use(t, map[string]string{"git": "aaa"})
			obj := f.declare(t, objective.Objective{
				Cadence:  &objective.Cadence{Every: "1h"},
				Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
			})
			ctx := context.Background()

			// One short of the breaker, so a deferral counted as a failure
			// trips it.
			st := f.state(t, obj.ID)
			st.ConsecutiveFailures = breaker - 1
			if err := f.store.SaveReconcileState(ctx, st); err != nil {
				t.Fatalf("seed consecutive failures: %v", err)
			}
			events, unsub := f.svc.hub.Subscribe(ctx, "obj:"+string(obj.ID))
			defer unsub()

			quotaPass(f, tc.retryAfter)
			f.pass(t, obj.ID, reconcile.TriggerManual)

			if got := f.loops.count(); got != 1 {
				t.Fatalf("the loop ran %d times, want 1", got)
			}
			history, err := f.svc.History(ctx, obj.ID, 10)
			if err != nil {
				t.Fatalf("history: %v", err)
			}
			if len(history) != 1 {
				t.Fatalf("recorded %d outcomes, want the one deferred pass", len(history))
			}
			o := history[0]
			if o.Deferred != waitingForQuota {
				t.Errorf("outcome.Deferred = %q, want %q", o.Deferred, waitingForQuota)
			}
			if o.Error != "" || o.Failed() {
				t.Errorf("outcome.Error = %q, want empty: out of quota is a state, not a failure", o.Error)
			}
			if tc.retryAfter > 0 {
				if want := f.clock.Add(tc.retryAfter); !o.DeferredUntil.Equal(want) {
					t.Errorf("outcome.DeferredUntil = %v, want %v: now plus the provider's retry-after", o.DeferredUntil, want)
				}
			} else if !o.DeferredUntil.IsZero() {
				t.Errorf("outcome.DeferredUntil = %v, want unset: the provider gave no time, and none is guessed", o.DeferredUntil)
			}
			if o.LoopID == "" {
				t.Error("the deferral lost the loop it came from")
			}

			after := f.state(t, obj.ID)
			if after.ConsecutiveFailures != breaker-1 {
				t.Errorf("consecutive_failures = %d, want %d unchanged", after.ConsecutiveFailures, breaker-1)
			}
			if after.Paused {
				t.Errorf("a pass out of quota tripped the breaker: %s", after.PausedReason)
			}
			if tc.retryAfter > 0 {
				if want := f.clock.Add(tc.retryAfter); after.NextDueAt == nil || !after.NextDueAt.Equal(want) {
					t.Errorf("next due = %v, want %v: the cadence must not schedule over the time the quota returns", after.NextDueAt, want)
				}
			}

			var reasons []string
			for _, e := range drainEvents(events) {
				switch e.Type {
				case event.TypeReconcileDeferred:
					r, _ := e.Payload["reason"].(string)
					reasons = append(reasons, r)
				case event.TypeReconcileFailed:
					t.Errorf("a pass out of quota published %s: %v", e.Type, e.Payload)
				}
			}
			if len(reasons) != 1 || reasons[0] != waitingForQuota {
				t.Errorf("%s reasons = %v, want one %q", event.TypeReconcileDeferred, reasons, waitingForQuota)
			}
		})
	}
}

// (b') A digest over one errored pass and one pass out of quota reports one
// failure, and counts the pass out of quota once under its own name. Read
// from the summary's JSON, as step 7's digest test reads its endings: where
// the count sits is the code's choice.
func TestTheDigestCountsAPassOutOfQuotaApartFromFailures(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 10})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		TwinID:   endingsTwin,
		Title:    "deliver the roadmap",
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	since := f.clock.Add(-time.Hour)

	// 1. Errored: the only failure.
	f.loops.status = objective.StatusFailed
	f.loops.failure = "the coding agent exited 1"
	f.loops.outcome = coreloop.RunErrored
	f.pass(t, obj.ID, reconcile.TriggerManual)
	// Past the hourly cadence's floor, which holds back even a forced pass.
	f.clock = f.clock.Add(2 * time.Hour)

	// 2. Out of quota.
	quotaPass(f, 2*time.Hour)
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
		t.Errorf("the digest reports %d failures, want 1: a pass out of quota is not one", sum.Failures)
	}

	raw, err := json.Marshal(sum)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	n, found := countUnder(tree, waitingForQuota)
	if !found {
		t.Fatalf("the digest does not count %q under its own name; summary: %s", waitingForQuota, raw)
	}
	if n != 1 {
		t.Errorf("the digest counts %d %q passes, want 1", n, waitingForQuota)
	}
}
