package loop

import (
	"context"
	"encoding/json"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
)

type plannedAction struct {
	CapabilityID string         `json:"capability"`
	Params       map[string]any `json:"params"`
	Reason       string         `json:"reason"`
	EnvID        string         `json:"env_id"`
}

type plan struct {
	Actions    []plannedAction `json:"actions"`
	Confidence float64         `json:"confidence"`
	Reasoning  string          `json:"reasoning"`
}

func stepReason(ctx context.Context, sc *stepContext, ws loop.WorldState) plan {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepReason),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	// 2. Build agent input
	memEntries := make([]coreagent.MemoryEntry, len(sc.memEntries))
	for i, e := range sc.memEntries {
		memEntries[i] = e
	}

	input := coreagent.Input{
		Objective:  sc.obj,
		WorldState: ws,
		Memory:     memEntries,
		Task: "Plan the next actions to make progress on this objective. " +
			"Return a JSON object with 'actions' (array of {capability, params, reason, env_id}), " +
			"'confidence' (0.0-1.0), and 'reasoning' (string).",
	}

	// 3. Call agent
	output, err := sc.agent.Run(ctx, input)

	var p plan
	if err == nil {
		// 4. Try to parse output as JSON plan
		if jsonErr := json.Unmarshal([]byte(output.Content), &p); jsonErr != nil {
			// Fallback: create default plan
			p = plan{
				Actions: []plannedAction{
					{
						CapabilityID: "reason.plan",
						Params:       map[string]any{"content": output.Content},
						Reason:       "Agent produced non-JSON output; wrapping as reasoning action",
					},
				},
				Confidence: 0.7,
				Reasoning:  output.Content,
			}
		}
	} else {
		// On error create a minimal plan
		p = plan{
			Actions: []plannedAction{
				{
					CapabilityID: "reason.plan",
					Params:       map[string]any{"error": err.Error()},
					Reason:       "Agent call failed",
				},
			},
			Confidence: 0.3,
			Reasoning:  "Agent call failed: " + err.Error(),
		}
	}

	// 5. Use output confidence if plan has none set
	if p.Confidence == 0 && err == nil {
		p.Confidence = output.Confidence
	}

	// 6. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":             string(loop.StepReason),
			"plan_action_count": len(p.Actions),
			"confidence":       p.Confidence,
		},
		Timestamp: time.Now().UTC(),
	})

	return p
}
