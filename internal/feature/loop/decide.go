package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
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

// effectiveThreshold returns the bounds-check threshold for this
// iteration, lowered by an operator's Decision.Modifications.RevisedConfidence
// when present (Phase 13.5 follow-up). The operator can only LOWER the
// threshold via modify, never raise it — raising would just escalate
// plans the agent already considered acceptable, which is pointless.
// The override is per-iteration: the agent's stated threshold returns
// on the next iteration.
func effectiveThreshold(authorityThreshold float64, mods *corecheckpoint.Modifications) float64 {
	if mods == nil || mods.RevisedConfidence == nil {
		return authorityThreshold
	}
	override := *mods.RevisedConfidence
	if override < authorityThreshold {
		return override
	}
	return authorityThreshold
}

func stepDecide(ctx context.Context, sc *stepContext, p plan, mods *corecheckpoint.Modifications) (plan, bool) {
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

	threshold := effectiveThreshold(authority.ConfidenceThreshold, mods)
	var capHistory map[string]float64

	if mods != nil && mods.RevisedConfidence != nil {
		// 2a. Operator override (Phase 13.5 follow-up): the operator has
		// reviewed the plan and attested a confidence value. Skip the
		// procedural-memory bias — the operator has the final word for
		// this iteration. Bounds are checked against the lowered
		// threshold; effectively, RevisedConfidence is both the asserted
		// confidence AND the per-iteration acceptance bar.
		p.Confidence = *mods.RevisedConfidence
		capHistory = map[string]float64{}
	} else {
		// 2a. Standard path: bias plan confidence from procedural memory
		// history (before authority check).
		adjustedConfidence, ch := biasConfidenceFromHistory(ctx, sc, p)
		p.Confidence = adjustedConfidence
		capHistory = ch
	}

	// 2. Check confidence threshold (lowered if operator override is set).
	if threshold > 0 && p.Confidence < threshold {
		escalate = true
		escalateReason = fmt.Sprintf("confidence %.2f below threshold %.2f",
			p.Confidence, threshold)
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
		// 3. Write the audit row first so the checkpoint payload can
		// reference its ID — reviewers deep-link from /checkpoints into
		// /audit without joining tables.
		auditPayload, _ := json.Marshal(map[string]any{
			"actions":                p.Actions,
			"confidence":             p.Confidence,
			"confidence_threshold":   authority.ConfidenceThreshold,
			"effective_threshold":    threshold,
			"max_autonomous":         authority.MaxAutonomousActions,
		})
		auditID := fmt.Sprintf("audit-%d", time.Now().UnixNano())
		_ = sc.svc.store.SaveToolEvent(ctx, storage.ToolEvent{
			ID:               auditID,
			ObjectiveID:      string(sc.obj.ID),
			AgentID:          string(sc.agentDef.ID),
			Success:          false,
			Confidence:       p.Confidence,
			Kind:             storage.ToolEventEscalation,
			EscalationReason: escalateReason,
			BoundsViolation:  true,
			PayloadJSON:      string(auditPayload),
		})

		// 4. Create checkpoint carrying the planner draft so reviewers
		// see what they're approving without leaving the response.
		options := []string{"approve", "reject", "modify"}
		summary := fmt.Sprintf("Loop %s iteration %d requires human decision: %s", sc.loopID, sc.iteration, escalateReason)

		actions := make([]corecheckpoint.Action, 0, len(p.Actions))
		var primaryCap string
		for i, a := range p.Actions {
			if i == 0 {
				primaryCap = a.CapabilityID
			}
			actions = append(actions, corecheckpoint.Action{
				CapabilityID: a.CapabilityID,
				Params:       a.Params,
				Reason:       a.Reason,
				EnvID:        a.EnvID,
			})
		}

		cp, err := sc.svc.cpSvc.Create(ctx,
			sc.obj.ID,
			sc.twinID,
			escalateReason,
			summary,
			options,
			featurecp.CreateOptions{
				Capability:   capability.CapabilityID(primaryCap),
				Confidence:   p.Confidence,
				Actions:      actions,
				AuditEventID: auditID,
			},
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
