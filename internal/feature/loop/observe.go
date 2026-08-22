package loop

import (
	"context"
	"sort"
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
	var blind []string

	for _, env := range sc.envs {
		obs, err := env.Observe(ctx, environment.ObservationQuery{Limit: 20})
		if err != nil {
			// An environment that could not look is named rather than dropped.
			// A bare `continue` here made "the calendar is down" and "the
			// calendar is empty" the same input to the planner, which is the
			// conflation the outer loop refused in Phase 20 and this one kept.
			blind = append(blind, string(env.ID()))
			continue
		}
		observations = append(observations, obs)
		if obs.Version != "" {
			versions = append(versions, obs.Version)
		}
		// An observation somebody outside this deployment wrote puts its
		// environment into evidence for every plan drafted from here on. It is
		// not cleared at the end of the iteration: material a planner has
		// already read cannot be un-read, and the actions it justifies may be
		// drafted an iteration later.
		if obs.Trust.IsThirdParty() {
			sc.evidence = sc.evidence.WithSource(string(env.ID()))
		}
	}

	sort.Strings(blind)

	// 3. Compute composite version SHA
	compositeVersion := vfs.SHA([]byte(strings.Join(versions, ",")))

	ws := loop.WorldState{
		Observations: observations,
		Version:      compositeVersion,
		Timestamp:    time.Now().UTC(),
		Blind:        blind,
	}

	// 4. Recall memory
	recallStart := time.Now()
	memEntries, err := sc.svc.memSvc.Recall(ctx, memory.Query{
		AgentID: sc.agentDef.ID,
		TwinID:  sc.twinID,
		Tiers:   []memory.Tier{memory.TierEpisodic, memory.TierSemantic},
		Query:   sc.obj.Title,
		TopK:    5,
	})
	if err == nil {
		sc.memEntries = memEntries
		sc.svc.otel.RecordMemoryRecall("episodic+semantic", len(memEntries), time.Since(recallStart).Milliseconds())
	}

	// 5. Emit step completed
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":                string(loop.StepObserve),
			"obs_count":           len(observations),
			"world_state_version": compositeVersion,
			"blind":               blind,
			"third_party_sources": sc.evidence.ThirdParty,
		},
		Timestamp: time.Now().UTC(),
	})

	return ws
}
