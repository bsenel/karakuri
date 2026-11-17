package reconcile

// Phase 40 step 9, slice 1: streams share the quota in a declared order.
//
// What the code does today, read before writing these (2026-11-17): Tick asks
// ListDueReconcileStates for up to MaxConcurrent*4 due rows, ordered by
// next_due_at ascending, and dispatches them in that order; dispatch takes a
// slot from the inflight channel (sized MaxConcurrent) or leaves the objective
// due for the next tick. So the most overdue objective gets the slot, whatever
// its priority.

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// When more objectives are due than there are free slots, the slots go to the
// highest priority first and, among equals, to the most overdue. An owner's
// override of the declared priorities changes which one runs.
func TestDueStreamsGetSlotsInPriorityOrder(t *testing.T) {
	type stream struct {
		id       objective.ObjectiveID
		priority int
		overdue  time.Duration
	}
	// The highest priority is the least overdue, so overdue order alone
	// picks the wrong one.
	declared := []stream{
		{"delivery", 30, time.Minute},
		{"ux", 20, 2 * time.Hour},
		{"discovery", 10, 3 * time.Hour},
	}
	for _, tc := range []struct {
		name     string
		declared []stream
		// override is the priorities the owner sets after declaring.
		override map[objective.ObjectiveID]int
		want     objective.ObjectiveID
	}{
		{name: "declared order", declared: declared, want: "delivery"},
		{name: "owner's override", declared: declared, override: map[objective.ObjectiveID]int{"ux": 50}, want: "ux"},
		{name: "equal priority, most overdue first", declared: []stream{
			{"delivery", 0, time.Minute},
			{"ux", 0, 2 * time.Hour},
			{"discovery", 0, 3 * time.Hour},
		}, want: "discovery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{MaxConcurrent: 1})
			f.use(t, map[string]string{"git": "aaa"})
			f.loops.gate = make(chan struct{})
			ctx := context.Background()

			for _, s := range tc.declared {
				obj := f.declare(t, objective.Objective{
					ID:       s.id,
					Priority: s.priority,
					Cadence:  &objective.Cadence{Every: "1h"},
					Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
				})
				if p, ok := tc.override[s.id]; ok {
					obj.Priority = p
					if err := f.store.SaveObjective(ctx, obj); err != nil {
						t.Fatalf("owner's override: %v", err)
					}
				}
				// Due, and due for a full reconcile, since overdue ago.
				st := f.state(t, s.id)
				at := f.clock.Add(-s.overdue)
				st.NextDueAt = &at
				st.NextReconcileAt = &at
				if err := f.store.SaveReconcileState(ctx, st); err != nil {
					t.Fatalf("seed due state: %v", err)
				}
			}

			f.svc.Tick(ctx)
			if !waitFor(5*time.Second, func() bool { return f.loops.count() >= 1 }) {
				t.Fatal("no pass reached the loop")
			}
			// The one slot is held; nothing else may start.
			time.Sleep(50 * time.Millisecond)
			f.loops.mu.Lock()
			ran := append([]objective.ObjectiveID(nil), f.loops.ran...)
			f.loops.mu.Unlock()
			close(f.loops.gate)
			if !waitFor(5*time.Second, func() bool {
				f.svc.mu.Lock()
				defer f.svc.mu.Unlock()
				return len(f.svc.running) == 0
			}) {
				t.Fatal("the pass did not end")
			}

			if len(ran) != 1 {
				t.Fatalf("passes started with one slot = %v, want one", ran)
			}
			if ran[0] != tc.want {
				t.Errorf("the slot went to %q, want %q", ran[0], tc.want)
			}
		})
	}
}
