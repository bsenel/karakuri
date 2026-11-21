package reconcile

// Phase 40 step 9, slice 3: the order does not starve the fixed cadence. A
// scheduled discovery cycle that is due takes the first slot after the
// delivery pass in flight has ended, ahead of delivery's next pass; it waits
// for a running pass to end, never for the pipeline to drain.
//
// What the code does today, read before writing these (2026-11-17):
//   - A pass's finish() sets LastReconciledAt to the pass's end and re-arms
//     next_due_at from the cadence measured from there. A continuous stream,
//     one whose cadence is short against its pass, is due again right after
//     each pass; a fixed-cadence one is due once per declared interval.
//   - Tick hands slots out in priority order (slice 1), so with one slot a
//     continuous delivery stream that is due again after every pass is ahead
//     of a due discovery cycle on every tick, for as long as work remains.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

const (
	starveDelivery  objective.ObjectiveID = "delivery"
	starveDiscovery objective.ObjectiveID = "discovery"
)

// With one slot, delivery (highest priority, continuous) is running when a
// discovery cycle (lower priority, hourly) comes due. When delivery's pass
// ends, the next pass is discovery's, and delivery's next pass comes after it.
// The control: with no discovery cycle due, delivery runs again.
func TestADueScheduledCycleIsNotStarvedByDelivery(t *testing.T) {
	for _, tc := range []struct {
		name string
		// discoveryDue is when the discovery cycle comes due, from the start
		// of delivery's pass, which ends at +30m.
		discoveryDue time.Duration
		want         []objective.ObjectiveID
	}{
		{name: "cycle due during the pass", discoveryDue: 10 * time.Minute,
			want: []objective.ObjectiveID{starveDiscovery, starveDelivery}},
		{name: "no cycle due", discoveryDue: 3 * time.Hour,
			want: []objective.ObjectiveID{starveDelivery, starveDelivery}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A one-minute floor, so delivery is due again a minute after its
			// pass rather than after the deployment's default ten.
			f := newFixture(t, Config{MaxConcurrent: 1, DefaultMinInterval: time.Minute})
			f.use(t, map[string]string{"git": "aaa"})
			ctx := context.Background()
			t0 := f.clock

			act := &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct}
			// Delivery is continuous: due again a minute after each pass.
			f.declare(t, objective.Objective{ID: starveDelivery, Priority: 50,
				Cadence: &objective.Cadence{Every: "1m"}, Autonomy: act})
			// Discovery runs on a fixed hourly cadence.
			f.declare(t, objective.Objective{ID: starveDiscovery, Priority: 10,
				Cadence: &objective.Cadence{Every: "1h"}, Autonomy: act})
			st := f.state(t, starveDiscovery)
			at := t0.Add(tc.discoveryDue)
			st.NextDueAt, st.NextReconcileAt = &at, &at
			if err := f.store.SaveReconcileState(ctx, st); err != nil {
				t.Fatalf("seed discovery's due time: %v", err)
			}

			// Delivery's pass starts at t0 and holds the one slot.
			gate := make(chan struct{})
			f.loops.mu.Lock()
			f.loops.gate = gate
			f.loops.mu.Unlock()
			f.svc.Tick(ctx)
			if !waitFor(5*time.Second, func() bool { return f.loops.count() == 1 }) {
				t.Fatal("delivery's pass did not start")
			}

			// Twenty minutes in, discovery may be due; the slot is held, so
			// it waits for the running pass and starts nothing beside it.
			f.clock = t0.Add(20 * time.Minute)
			f.svc.Tick(ctx)
			time.Sleep(50 * time.Millisecond)
			if ran := f.ranSince(0); !slices.Equal(ran, []objective.ObjectiveID{starveDelivery}) {
				t.Fatalf("while delivery's pass held the slot, the passes started were %v, want only %q's", ran, starveDelivery)
			}

			// The pass ends at +30m.
			f.clock = t0.Add(30 * time.Minute)
			f.loops.mu.Lock()
			f.loops.gate = nil
			f.loops.mu.Unlock()
			close(gate)
			if !waitFor(5*time.Second, f.idle) {
				t.Fatal("delivery's pass did not end")
			}

			// Delivery is due again a minute later; one tick per slot.
			var got []objective.ObjectiveID
			for _, at := range []time.Duration{31 * time.Minute, 32 * time.Minute} {
				f.clock = t0.Add(at)
				got = append(got, tickAndSettle(t, f)...)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("after delivery's pass ended, the passes were %v, want %v", got, tc.want)
			}
		})
	}
}
