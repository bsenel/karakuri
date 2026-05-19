package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
)

func stepDecide(ctx context.Context, sc *stepContext, p plan) (plan, bool) {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepDecide),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	authority := sc.agentDef.Authority
	escalate := false
	escalateReason := ""

	// 2. Check confidence threshold
	if authority.ConfidenceThreshold > 0 && p.Confidence < authority.ConfidenceThreshold {
		escalate = true
		escalateReason = fmt.Sprintf("confidence %.2f below threshold %.2f",
			p.Confidence, authority.ConfidenceThreshold)
	}

	// Check if any action requires approval
	if !escalate {
		approvalSet := make(map[capability.CapabilityID]struct{}, len(authority.RequiresApprovalFor))
		for _, cap := range authority.RequiresApprovalFor {
			approvalSet[cap] = struct{}{}
		}
		for _, action := range p.Actions {
			if _, requires := approvalSet[capability.CapabilityID(action.CapabilityID)]; requires {
				escalate = true
				escalateReason = fmt.Sprintf("action %q requires approval", action.CapabilityID)
				break
			}
		}
	}

	// Trim actions if exceeds max autonomous
	if authority.MaxAutonomousActions > 0 && len(p.Actions) > authority.MaxAutonomousActions {
		p.Actions = p.Actions[:authority.MaxAutonomousActions]
	}

	paused := false

	if escalate {
		// 3. Create checkpoint
		options := []string{"approve", "reject", "modify"}
		summary := fmt.Sprintf("Loop %s iteration %d requires human decision: %s", sc.loopID, sc.iteration, escalateReason)

		cp, err := sc.svc.cpSvc.Create(ctx,
			sc.obj.ID,
			sc.twinID,
			escalateReason,
			summary,
			options,
		)

		cpID := ""
		if err == nil {
			cpID = cp.ID
		}

		// Update state
		sc.state.mu.Lock()
		sc.state.status.Paused = true
		cpIDCopy := cpID
		sc.state.result.CheckpointID = &cpIDCopy
		sc.state.mu.Unlock()

		// Emit checkpoint event
		sc.svc.hub.Publish(ctx, event.Event{
			Type:        event.TypeCheckpoint,
			ObjectiveID: string(sc.obj.ID),
			Payload: map[string]any{
				"checkpoint_id": cpID,
				"reason":        escalateReason,
				"loop_id":       sc.loopID,
			},
			Timestamp: time.Now().UTC(),
		})

		// Block until decision arrives
		select {
		case <-ctx.Done():
		case <-sc.state.decisionCh:
		}

		sc.state.mu.Lock()
		sc.state.status.Paused = false
		sc.state.mu.Unlock()

		paused = true
	}

	// 4. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepDecide),
			"escalated": escalate,
		},
		Timestamp: time.Now().UTC(),
	})

	return p, paused
}
