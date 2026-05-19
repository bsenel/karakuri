package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/storage"
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
		// a. Find matching environment
		var targetEnv environment.Environment
		if action.EnvID != "" {
			for _, env := range sc.envs {
				if string(env.ID()) == action.EnvID {
					targetEnv = env
					break
				}
			}
		}
		if targetEnv == nil && len(sc.envs) > 0 {
			targetEnv = sc.envs[0]
		}

		params := action.Params
		if params == nil {
			params = make(map[string]any)
		}

		// b. Worktree for code-writing capabilities
		capLower := strings.ToLower(action.CapabilityID)
		if strings.HasSuffix(capLower, ".write_code") || strings.HasSuffix(capLower, ".write_test") {
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
			// No environment available
			result = environment.ActionResult{
				Success:    true,
				StateDelta: map[string]any{"note": "no environment; action recorded only"},
			}
			sc.svc.hub.Publish(ctx, event.Event{
				Type:        event.TypeAdapterSkipped,
				ObjectiveID: string(sc.obj.ID),
				Payload:     map[string]any{"capability": action.CapabilityID, "reason": "no environment"},
				Timestamp:   time.Now().UTC(),
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
