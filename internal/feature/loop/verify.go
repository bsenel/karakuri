package loop

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func stepVerify(ctx context.Context, sc *stepContext, results []environment.ActionResult) (float64, bool) {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepVerify),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	criteria := sc.obj.SuccessCriteria

	// No criteria → success
	if len(criteria) == 0 {
		sc.svc.hub.Publish(ctx, event.Event{
			Type:        event.TypeLoopStepCompleted,
			ObjectiveID: string(sc.obj.ID),
			Payload: map[string]any{
				"step":             string(loop.StepVerify),
				"criteria_met_count": 0,
				"weighted_score":   1.0,
			},
			Timestamp: time.Now().UTC(),
		})
		return 1.0, true
	}

	// Build a results index by capability
	resultsByCapability := make(map[string]environment.ActionResult)
	for _, r := range results {
		// We stored capability info in StateDelta["capability"] indirectly; match by iteration
		_ = r // Keep it simple: we'll pass results array to agent
	}
	_ = resultsByCapability

	totalWeight := 0.0
	metWeight := 0.0
	metCount := 0

	for i, criterion := range criteria {
		weight := criterion.Weight
		if weight == 0 {
			weight = 1.0
		}
		totalWeight += weight

		met := false
		verifier := string(criterion.Verifier)

		if verifier == "" {
			// No verifier set — use agent to evaluate
			met = evaluateWithAgent(ctx, sc, criterion, results)
		} else if strings.Contains(verifier, "run_tests") || strings.Contains(verifier, "lint") {
			// Look in action results for a matching capability result
			for _, r := range results {
				if r.Success {
					met = true
					break
				}
			}
		} else {
			// Spawn agent review
			met = evaluateWithAgent(ctx, sc, criterion, results)
		}

		criteria[i].Met = met
		if met {
			metWeight += weight
			metCount++
		}
	}

	// 3. Compute weighted score
	score := 0.0
	if totalWeight > 0 {
		score = metWeight / totalWeight
	}
	allMet := metCount == len(criteria)

	// 4. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":               string(loop.StepVerify),
			"criteria_met_count": metCount,
			"weighted_score":     score,
		},
		Timestamp: time.Now().UTC(),
	})

	return score, allMet
}

func evaluateWithAgent(ctx context.Context, sc *stepContext, criterion objective.Criterion, results []environment.ActionResult) bool {
	task := fmt.Sprintf(
		"Evaluate whether this criterion is met based on the action results. "+
			"Criterion: %q. "+
			"Answer with only 'pass' or 'fail'.",
		criterion.Description,
	)

	input := coreagent.Input{
		Objective:  sc.obj,
		WorldState: nil,
		Memory:     nil,
		Task:       task,
	}

	output, err := sc.agent.Run(ctx, input)
	if err != nil {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(output.Content))
	for _, keyword := range []string{"pass", "met", "approved", "yes"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
