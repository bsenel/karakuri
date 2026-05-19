package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func (s *serviceImpl) runWatchMode(ctx context.Context, state *loopState, obj objective.Objective, envs []environment.Environment, sc *stepContext) {
	// Subscribe to all environments
	var channels []<-chan environment.EnvironmentEvent
	for _, env := range envs {
		ch, err := env.Subscribe(ctx, environment.EventFilter{})
		if err == nil && ch != nil {
			channels = append(channels, ch)
		}
	}

	// Update state to indicate watching
	state.mu.Lock()
	state.status.Paused = false
	state.mu.Unlock()

	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(obj.ID),
		Payload:     map[string]any{"step": "watch", "env_count": len(channels)},
		Timestamp:   time.Now().UTC(),
	})

	// Track last-seen SHA per environment to detect real changes.
	lastSHA := make(map[environment.EnvironmentID]string)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, env := range envs {
				snap, err := env.Snapshot(ctx)
				if err != nil || snap.SHA == "" {
					continue
				}
				prev := lastSHA[snap.EnvID]
				if snap.SHA == prev {
					continue // no change
				}
				lastSHA[snap.EnvID] = snap.SHA
				if prev == "" {
					continue // first observation; establish baseline
				}
				s.emitWatchCheckpoint(ctx, state, obj, sc, "environment_changed",
					fmt.Sprintf("env %s changed: %s → %s", snap.EnvID, prev, snap.SHA))
			}
		}
	}
}

func (s *serviceImpl) emitWatchCheckpoint(ctx context.Context, state *loopState, obj objective.Objective, sc *stepContext, trigger, summary string) {
	cp, err := s.cpSvc.Create(ctx, obj.ID, obj.TwinID, trigger, summary, []string{"promote", "dismiss", "investigate"})
	if err != nil {
		return
	}

	state.mu.Lock()
	cpID := cp.ID
	state.result.CheckpointID = &cpID
	state.mu.Unlock()

	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeCheckpoint,
		ObjectiveID: string(obj.ID),
		Payload: map[string]any{
			"checkpoint_id": cp.ID,
			"trigger":       trigger,
			"summary":       summary,
			"loop_id":       state.id,
		},
		Timestamp: time.Now().UTC(),
	})
}
