package loop

import (
	"context"
	"testing"
)

// Phase 37 step 6: GET /loops/{id}/status says a stopped loop was stopped,
// by whom, why and when, so the console never shows it as completed.
func TestStatusOfAStoppedLoopSaysWhoStoppedItWhenAndWhy(t *testing.T) {
	env := newStopEnv(true)
	svc, req := stopFixture(t, env, unlimited)
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	env.waitStarted(t)

	if st, err := svc.Status(ctx, run.LoopID); err != nil || st.Stopped {
		t.Fatalf("Status before the stop = %+v, %v; want a running loop", st, err)
	}
	if _, err := svc.Stop(ctx, run.LoopID, stopPrincipal, stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	st, err := svc.Status(ctx, run.LoopID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Stopped || st.StoppedBy != stopPrincipal || st.StopReason != stopReason {
		t.Errorf("Status = stopped %v by %q because %q, want stopped by %q because %q",
			st.Stopped, st.StoppedBy, st.StopReason, stopPrincipal, stopReason)
	}
	if st.StoppedAt == nil || st.StoppedAt.IsZero() {
		t.Errorf("stopped at = %v, want the time of the stop", st.StoppedAt)
	}
}
