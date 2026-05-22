package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// biasConfidenceFromHistory adjusts plan confidence based on procedural memory success rates.
// Returns the adjusted confidence and a map of capability IDs with historical data.
func biasConfidenceFromHistory(ctx context.Context, sc *stepContext, p plan) (float64, map[string]float64) {
	confidence := p.Confidence
	capHistory := make(map[string]float64)

	for _, action := range p.Actions {
		rec, err := sc.svc.store.QueryProcedural(ctx, string(sc.agentDef.ID), action.CapabilityID)
		if err != nil {
			continue
		}
		total := rec.SuccessCount + rec.FailureCount
		if total == 0 {
			continue
		}
		successRate := float64(rec.SuccessCount) / float64(total)
		capHistory[action.CapabilityID] = successRate

		if successRate > 0.8 {
			confidence += 0.05
			if confidence > 1.0 {
				confidence = 1.0
			}
		} else if successRate < 0.3 {
			confidence -= 0.1
			if confidence < 0.0 {
				confidence = 0.0
			}
		}
	}

	return confidence, capHistory
}

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

	// 2a. Bias plan confidence from procedural memory history (before authority check)
	adjustedConfidence, capHistory := biasConfidenceFromHistory(ctx, sc, p)
	p.Confidence = adjustedConfidence

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

		// 3b. Write an audit record so `krk audit` can surface this
		// escalation later without scraping checkpoint history. The payload
		// captures the planner's draft (actions + confidence) at the moment
		// of escalation, which is what a reviewer needs to judge whether
		// the bounds were tuned correctly.
		auditPayload, _ := json.Marshal(map[string]any{
			"actions":              p.Actions,
			"confidence":           p.Confidence,
			"confidence_threshold": authority.ConfidenceThreshold,
			"max_autonomous":       authority.MaxAutonomousActions,
			"checkpoint_id":        cpID,
		})
		_ = sc.svc.store.SaveToolEvent(ctx, storage.ToolEvent{
			ID:               fmt.Sprintf("audit-%d", time.Now().UnixNano()),
			ObjectiveID:      string(sc.obj.ID),
			AgentID:          string(sc.agentDef.ID),
			Success:          false,
			Confidence:       p.Confidence,
			Kind:             storage.ToolEventEscalation,
			EscalationReason: escalateReason,
			BoundsViolation:  true,
			PayloadJSON:      string(auditPayload),
		})

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

		paused = true
	}

	// 4. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":                string(loop.StepDecide),
			"escalated":           escalate,
			"adjusted_confidence": p.Confidence,
			"capability_history":  capHistory,
		},
		Timestamp: time.Now().UTC(),
	})

	return p, paused
}
