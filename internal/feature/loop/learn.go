package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func stepLearn(ctx context.Context, sc *stepContext, ws loop.WorldState, p plan, results []environment.ActionResult, score float64) {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepLearn),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	// 2. Save LoopIteration to storage
	inputJSON, _ := json.Marshal(map[string]any{
		"world_state": ws,
		"plan":        p,
	})
	outputJSON, _ := json.Marshal(map[string]any{
		"results": results,
		"score":   score,
	})
	_ = sc.svc.store.SaveLoopIteration(ctx, storage.LoopIteration{
		ID:          fmt.Sprintf("li-%d", time.Now().UnixNano()),
		ObjectiveID: string(sc.obj.ID),
		Number:      sc.iteration,
		Step:        string(loop.StepLearn),
		InputJSON:   string(inputJSON),
		OutputJSON:  string(outputJSON),
		CreatedAt:   time.Now().UTC(),
	})

	// 3. Write episodic memory entry
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	episodicContent := fmt.Sprintf(
		"Iteration %d: executed %d actions (%d successful), score=%.2f, objective=%q",
		sc.iteration, len(results), successCount, score, sc.obj.Title,
	)
	_ = sc.svc.memSvc.Store(ctx, memory.Entry{
		ID:         fmt.Sprintf("ep-%d", time.Now().UnixNano()),
		AgentID:    sc.agentDef.ID,
		TwinID:     sc.twinID,
		Tier:       string(memory.TierEpisodic),
		Domain:     sc.obj.Domain,
		Content:    episodicContent,
		Confidence: score,
		CreatedAt:  time.Now().UTC(),
	})

	// 4. Upsert procedural memory for each action result
	memoriesWritten := 1 // episodic entry above
	for i, action := range p.Actions {
		var actionResult environment.ActionResult
		if i < len(results) {
			actionResult = results[i]
		}

		confidence := 0.8
		if !actionResult.Success {
			confidence = 0.2
		}

		// Read existing procedural record first, then increment counts via Store
		_ = sc.svc.memSvc.Store(ctx, memory.Entry{
			ID:         fmt.Sprintf("proc-%d-%d", time.Now().UnixNano(), i),
			AgentID:    coreagent.AgentID(sc.agentDef.ID),
			TwinID:     sc.twinID,
			Tier:       string(memory.TierProcedural),
			Domain:     action.CapabilityID, // capability ID stored in Domain for procedural
			Content:    fmt.Sprintf("capability=%s success=%v", action.CapabilityID, actionResult.Success),
			Confidence: confidence,
			CreatedAt:  time.Now().UTC(),
		})
		memoriesWritten++
	}

	// 5. Trigger memory consolidation (promotes high-confidence episodic entries to semantic)
	_ = sc.svc.memSvc.Consolidate(ctx, sc.agentDef.ID, 20)

	// 6. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":                  string(loop.StepLearn),
			"memory_entries_written": memoriesWritten,
		},
		Timestamp: time.Now().UTC(),
	})

	// 7. Emit iteration done
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopIterationDone,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"iteration":    sc.iteration,
			"criteria_met": score,
		},
		Timestamp: time.Now().UTC(),
	})
}
