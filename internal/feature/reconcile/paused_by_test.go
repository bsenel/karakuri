package reconcile

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 37 step 6: a paused objective says who paused it and when, beside
// why, so the console can show it, and a resume clears all three.
func TestPauseRecordsWhoPausedItAndWhenAndResumeClearsThem(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	if err := f.svc.Pause(context.Background(), obj.ID, "alice", "started against a merged branch"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	st := f.state(t, obj.ID)
	if st.PausedBy != "alice" {
		t.Errorf("paused by = %q, want alice", st.PausedBy)
	}
	if st.PausedAt == nil || st.PausedAt.IsZero() {
		t.Errorf("paused at = %v, want the time of the pause", st.PausedAt)
	}

	if err := f.svc.Resume(context.Background(), obj.ID, "alice"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	st = f.state(t, obj.ID)
	if st.PausedBy != "" || st.PausedAt != nil {
		t.Errorf("after resume: paused by %q at %v, want neither", st.PausedBy, st.PausedAt)
	}
}

// A pause that cancels a pass in flight is re-applied by the ending pass from
// its own, older copy of the state; who paused it must survive that.
func TestPauseInFlightKeepsWhoPausedIt(t *testing.T) {
	s := startBlockedPass(t)
	s.pause(t)
	s.awaitEnded(t)

	st := s.state(t, s.obj.ID)
	if st.PausedBy != "operator" {
		t.Errorf("paused by = %q after the stopped pass ended, want operator", st.PausedBy)
	}
	if st.PausedAt == nil {
		t.Error("paused at is unset after the stopped pass ended")
	}
}
