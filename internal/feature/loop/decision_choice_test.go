package loop

import (
	"context"
	"errors"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// POST /loops/{id}/resume delivers the decision to the waiting runner before
// it records it, and the runner reads any choice other than reject or modify
// as approve. An unrecognised choice must therefore be refused before it is
// delivered, and the loop must still be waiting afterwards.
func TestResumeRefusesAChoiceThatIsNotApproveRejectOrModify(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	ctx := context.Background()
	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitAtCheckpoint(t, svc, run.LoopID)
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), run.LoopID, stopPrincipal, "test cleanup") })

	_, err = svc.Resume(ctx, run.LoopID, corecheckpoint.Decision{Choice: "deny", Approver: stopPrincipal})

	if !errors.Is(err, coreerrors.ErrInvalidInput) {
		t.Fatalf("Resume(deny) = %v, want an ErrInvalidInput refusal", err)
	}
	time.Sleep(stopSettle)
	if n := env.startCount(); n != 0 {
		t.Errorf("a refused decision let the plan run: %d action(s) started", n)
	}
	waitAtCheckpoint(t, svc, run.LoopID)
}
