package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

func stepAct(ctx context.Context, sc *stepContext, p plan) []environment.ActionResult {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepAct),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	results := make([]environment.ActionResult, 0, len(p.Actions))
	successCount := 0

	for i, action := range p.Actions {
		// a. Find matching environment. If the agent specified an
		// EnvID, it must match an environment registered by the twin's
		// active domain packs. Previously an unmatched EnvID fell
		// through to envs[0] — silently routing the action to whatever
		// happened to be first in the slice, almost always wrong, and
		// the main reason the Phase 13.5 dogfood loop "completed" with
		// every action a noop. Now an unmatched EnvID fails honestly.
		var targetEnv environment.Environment
		if action.EnvID != "" {
			for _, env := range sc.envs {
				if string(env.ID()) == action.EnvID {
					targetEnv = env
					break
				}
			}
		} else if len(sc.envs) == 1 {
			// No EnvID given but only one env registered — unambiguous.
			targetEnv = sc.envs[0]
		}

		params := action.Params
		if params == nil {
			params = make(map[string]any)
		}

		// a2. Charge the twin's daily allowance for this capability.
		//
		// Before the action rather than after, because the point of the tier is
		// to stop a misconfigured watcher hammering an external service — most
		// capabilities are not model calls, they reach GitHub or Linear, and a
		// charge levied afterwards would have already made the call.
		//
		// A refusal skips the action and records why, rather than failing the
		// iteration: the loop can still make progress on whatever else it
		// planned, and the operator sees a bounded twin rather than a broken one.
		if !sc.svc.allowCapability(ctx, sc, action.CapabilityID, i) {
			results = append(results, environment.ActionResult{
				Success: false,
				Error:   "capability quota exhausted for the day",
			})
			continue
		}

		// b. Worktree for capabilities that declare they write files.
		//
		// Asked of the capability registry rather than matched on the name:
		// the suffix test gave a worktree to two capabilities that had no
		// implementation and withheld one from delegate_to_cli, which is the
		// only one that can actually write. See ADR 019.
		if sc.svc.needsWorkspace(action.CapabilityID) {
			taskID := fmt.Sprintf("%s-%d", sc.loopID[:8], i)
			wt, err := sc.svc.wt.Create(ctx, git.WorktreeOptions{
				ObjectiveID: sc.obj.ID,
				TaskID:      taskID,
			})
			if err == nil {
				params["worktree_path"] = wt.Path
				params["branch"] = wt.Branch
				// Persist worktree record
				_ = sc.svc.store.SaveWorktree(ctx, storage.Worktree{
					TaskID:      wt.TaskID,
					ObjectiveID: string(wt.ObjectiveID),
					Path:        wt.Path,
					Branch:      wt.Branch,
					CreatedAt:   wt.CreatedAt,
				})
				sc.svc.hub.Publish(ctx, event.Event{
					Type:        event.TypeWorktreeCreated,
					ObjectiveID: string(sc.obj.ID),
					Payload:     map[string]any{"task_id": taskID, "path": wt.Path, "branch": wt.Branch},
					Timestamp:   time.Now().UTC(),
				})
			}
		}

		var result environment.ActionResult
		if targetEnv != nil {
			var err error
			result, err = targetEnv.Act(ctx, environment.Action{
				CapabilityID: capability.CapabilityID(action.CapabilityID),
				Params:       params,
			})
			if err != nil {
				result = environment.ActionResult{
					Success: false,
					Error:   err.Error(),
				}
			}
		} else {
			// No environment matched the agent's EnvID (or no EnvID
			// was given and the twin has multiple envs registered).
			// Used to silently succeed with Success=true; now fails
			// honestly so the audit trail and verify step see the
			// gap instead of treating it as work done.
			available := make([]string, 0, len(sc.envs))
			for _, env := range sc.envs {
				available = append(available, string(env.ID()))
			}
			result = environment.ActionResult{
				Success: false,
				Error:   fmt.Sprintf("no environment matches env_id=%q (available: %v)", action.EnvID, available),
				StateDelta: map[string]any{
					"capability": action.CapabilityID,
					"status":     "unrouted",
					"env_id":     action.EnvID,
					"available":  available,
				},
			}
			sc.svc.hub.Publish(ctx, event.Event{
				Type:        event.TypeAdapterSkipped,
				ObjectiveID: string(sc.obj.ID),
				Payload: map[string]any{
					"capability": action.CapabilityID,
					"reason":     "unrouted",
					"env_id":     action.EnvID,
					"available":  available,
				},
				Timestamp: time.Now().UTC(),
			})
		}

		// d. Emit artifact_written if blobs produced
		if len(result.ArtifactSHAs) > 0 {
			sc.svc.hub.Publish(ctx, event.Event{
				Type:        event.TypeArtifactWritten,
				ObjectiveID: string(sc.obj.ID),
				Payload:     map[string]any{"shas": result.ArtifactSHAs, "capability": action.CapabilityID},
				Timestamp:   time.Now().UTC(),
			})
		}

		if result.Success {
			successCount++
		}

		// f. Save ToolEvent
		payloadJSON, _ := json.Marshal(map[string]any{"params": params, "result": result})
		agentIDStr := string(sc.agentDef.ID)
		envAdapter := ""
		if targetEnv != nil {
			envAdapter = string(targetEnv.ID())
		}
		_ = sc.svc.store.SaveToolEvent(ctx, storage.ToolEvent{
			ID:          fmt.Sprintf("te-%d-%d", time.Now().UnixNano(), i),
			ObjectiveID: string(sc.obj.ID),
			AgentID:     agentIDStr,
			Capability:  action.CapabilityID,
			Adapter:     envAdapter,
			Success:     result.Success,
			Confidence:  p.Confidence,
			PayloadJSON: string(payloadJSON),
			CreatedAt:   time.Now().UTC(),
		})

		// g. And what it cost. Attributed to the objective rather than the twin,
		// because "which piece of work spent this" is the question a bill
		// prompts — the twin is still the subject that pays, and its containers
		// are what a per-team report groups on.
		sc.svc.costs.Record(ctx, karakuriquota.Spend{
			TwinID:       sc.twinID,
			ResourceType: "objective",
			ResourceID:   string(sc.obj.ID),
			Provider:     envAdapter,
			Units:        1,
			UnitKind:     cost.UnitCalls,
		})

		results = append(results, result)
	}

	// 3. Emit step completed
	successRate := 0.0
	if len(p.Actions) > 0 {
		successRate = float64(successCount) / float64(len(p.Actions))
	}
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":         string(loop.StepAct),
			"action_count": len(p.Actions),
			"success_rate": successRate,
		},
		Timestamp: time.Now().UTC(),
	})

	return results
}

// needsWorkspace reports whether a capability declared that it writes files.
//
// Unknown capabilities get no workspace: a capability the registry has never
// heard of is one no pack declared, and provisioning a git worktree for a
// name a model invented would create a branch per hallucination.
func (s *serviceImpl) needsWorkspace(capID string) bool {
	if s.capReg == nil {
		return false
	}
	cap, ok := s.capReg.Get(capability.CapabilityID(capID))
	return ok && cap.NeedsWorkspace
}
