package loop

// Phase 37 step 5, slice 2 (d): a checkpoint closed by a stop is not a
// person's rejection.
//
// What the code does today (read on 2026-11-09):
//   - closeStopped (stop.go) closes the checkpoint a stopped loop was waiting
//     at through checkpoint.Service.Record with Choice "reject" and the
//     stopping principal as Approver.
//   - Record (feature/checkpoint/service.go) writes a ToolEventRejection row
//     for it with an empty EscalationReason, the same row a person's
//     rejection leaves.
//   - Reader.readAudit (platform/telemetry/reader.go) counts as Rejections
//     exactly the rejection rows with an empty EscalationReason, so the stop
//     reads as a rejection, and EscalationStats.ApprovalRate() and the
//     escalation_approval_rate verifier fall with every stop.
//
// The test lives here rather than beside the reader's tests: those seed rows
// by hand, and this one must drive the real stop path, which only the loop
// package's tests can reach, and read it through the real reader, which they
// can import without a cycle.

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	platformtelemetry "github.com/bsenel/karakuri/internal/platform/telemetry"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

func TestACheckpointClosedByAStopIsNoOnesRejection(t *testing.T) {
	env := newStopEnv(false)
	// No action may be taken on the agent's own authority, so each loop's
	// first plan escalates to a checkpoint.
	svc, req := stopFixture(t, env, coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	ctx := context.Background()

	// The first loop's checkpoint is approved by a person.
	approved, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run (approved loop): %v", err)
	}
	approvedCP := waitAtCheckpoint(t, svc, approved.LoopID)
	if err := svc.cpSvc.Resolve(ctx, approvedCP, corecheckpoint.Decision{
		Choice: "approve", Approver: "user:grace", Note: "go ahead",
	}); err != nil {
		t.Fatalf("approve the first checkpoint: %v", err)
	}

	// The second loop, on its own objective over the same store, is stopped
	// while it waits at its checkpoint.
	second := req.Objective
	second.ID = objective.ObjectiveID(string(req.Objective.ID) + "-stopped")
	if err := svc.store.SaveObjective(ctx, second); err != nil {
		t.Fatalf("save second objective: %v", err)
	}
	stoppedReq := req
	stoppedReq.Objective = second
	stopped, err := svc.Run(ctx, stoppedReq)
	if err != nil {
		t.Fatalf("Run (stopped loop): %v", err)
	}
	stoppedCP := waitAtCheckpoint(t, svc, stopped.LoopID)
	if _, err := svc.Stop(ctx, stopped.LoopID, stopPrincipal, stopReason); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitStoredCompleted(t, svc.store, stopped.LoopID)

	// The stop still closes the checkpoint as rejected, and an approval
	// arriving afterwards is still refused.
	cp, err := svc.store.GetCheckpoint(ctx, stoppedCP)
	if err != nil {
		t.Fatalf("get stopped checkpoint: %v", err)
	}
	if cp.Status != corecheckpoint.StatusResolved || cp.Decision == nil || cp.Decision.Choice != "reject" {
		t.Errorf("stopped loop's checkpoint = status %q decision %+v, want resolved as rejected", cp.Status, cp.Decision)
	}
	if err := svc.cpSvc.Resolve(ctx, stoppedCP, corecheckpoint.Decision{
		Choice: "approve", Approver: "user:grace", Note: "late",
	}); err == nil {
		t.Error("an approval of the stopped loop's checkpoint was accepted; it must be refused")
	}

	snap, err := platformtelemetry.New(svc.store, karakuriquota.Deps{}).Snapshot(ctx, coretelemetry.Query{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Escalation.Approvals != 1 {
		t.Errorf("approvals = %d, want 1: one person approved one checkpoint", snap.Escalation.Approvals)
	}
	if snap.Escalation.Rejections != 0 {
		t.Errorf("rejections = %d, want 0: the only rejected checkpoint was closed by a stop, "+
			"and nobody judged its plan; closeStopped records it through checkpoint.Service.Record "+
			"as a rejection row with an empty EscalationReason, which readAudit counts as a person's",
			snap.Escalation.Rejections)
	}
	if got := snap.Escalation.ApprovalRate(); got != 1.0 {
		t.Errorf("ApprovalRate() = %v, want exactly 1.0: a stop is not a judgement on the plan", got)
	}
}
