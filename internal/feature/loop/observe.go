package loop

import (
	"context"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/vfs"
)

func stepObserve(ctx context.Context, sc *stepContext) loop.WorldState {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":         string(loop.StepObserve),
			"objective_id": string(sc.obj.ID),
			"iteration":    sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	// 2. Fan out observations across all environments
	var observations []environment.Observation
	var versions []string

	for _, env := range sc.envs {
		obs, err := env.Observe(ctx, environment.ObservationQuery{Limit: 20})
		if err != nil {
			// Skip failed observations but continue
			continue
		}
		observations = append(observations, obs)
		if obs.Version != "" {
			versions = append(versions, obs.Version)
		}
	}

	// 3. Compute composite version SHA
	compositeVersion := vfs.SHA([]byte(strings.Join(versions, ",")))

	ws := loop.WorldState{
		Observations: observations,
		Version:      compositeVersion,
		Timestamp:    time.Now().UTC(),
	}

	// 4. Recall memory
	memEntries, err := sc.svc.memSvc.Recall(ctx, memory.Query{
		AgentID: sc.agentDef.ID,
		TwinID:  sc.twinID,
		Tiers:   []memory.Tier{memory.TierEpisodic, memory.TierSemantic},
		Query:   sc.obj.Title,
		TopK:    5,
	})
	if err == nil {
		sc.memEntries = memEntries
	}

	// 5. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":                string(loop.StepObserve),
			"obs_count":           len(observations),
			"world_state_version": compositeVersion,
		},
		Timestamp: time.Now().UTC(),
	})

	return ws
}
