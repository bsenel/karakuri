package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/delivery"
	"github.com/bsenel/karakuri/internal/feature/discovery"
	"github.com/bsenel/karakuri/internal/feature/strategy"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/executor"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Service struct {
	store     storage.StorageAdapter
	planner   *Planner
	scheduler *Scheduler
	factory   *platformagent.Factory
	strategy  *strategy.Service
	discovery *discovery.Service
	delivery  *delivery.Service
	events    *event.Hub
	otel      *observability.OTel
	exec      executor.Executor
}

func NewService(
	store storage.StorageAdapter,
	planner *Planner,
	scheduler *Scheduler,
	factory *platformagent.Factory,
	strat *strategy.Service,
	disc *discovery.Service,
	deliv *delivery.Service,
	events *event.Hub,
	otel *observability.OTel,
	exec executor.Executor,
) *Service {
	return &Service{
		store: store, planner: planner, scheduler: scheduler, factory: factory,
		strategy: strat, discovery: disc, delivery: deliv, events: events, otel: otel, exec: exec,
	}
}

func (s *Service) Run(ctx context.Context, sessionSHA string) (err error) {
	defer func() {
		if err != nil {
			_ = s.store.UpdateSessionState(context.Background(), sessionSHA, entity.StateFailed)
		}
	}()
	sess, err := s.store.GetSession(ctx, sessionSHA)
	if err != nil {
		return err
	}
	_ = s.store.UpdateSessionState(ctx, sessionSHA, entity.StatePlanning)
	manifest, err := s.store.GetManifest(ctx, sessionSHA)
	if err != nil {
		return err
	}
	plan, err := s.planner.Plan(ctx, string(sess.Mode), sessionSHA, manifest)
	if err != nil {
		return err
	}
	_ = s.store.UpdateSessionState(ctx, sessionSHA, entity.StateRunning)

	switch sess.Mode {
	case entity.ModeStrategy:
		return s.runStrategy(ctx, sessionSHA, sess.Input, plan)
	case entity.ModeDiscovery:
		return s.runDiscovery(ctx, sessionSHA, sess.Input, plan)
	case entity.ModeDelivery:
		return s.runDelivery(ctx, sessionSHA, sess.Input, plan)
	default:
		return fmt.Errorf("unsupported mode: %s", sess.Mode)
	}
}

func (s *Service) runStrategy(ctx context.Context, sessionSHA, input string, plan *ExecutionPlan) error {
	artifactMap := map[string][]string{
		"ProductManager":     {"prd"},
		"Architect":          {"design-doc", "adr"},
		"EngineeringManager": {"roadmap"},
	}
	for _, task := range plan.Tasks {
		content, err := s.strategy.RunRole(ctx, sessionSHA, task.Role, input)
		if err != nil {
			return err
		}
		for _, name := range artifactMap[task.Role] {
			if err := s.strategy.WriteArtifact(ctx, sessionSHA, name, task.Role, content); err != nil {
				return err
			}
			_ = s.events.Publish(ctx, event.Event{
				Type: event.ArtifactWritten, SessionSHA: sessionSHA,
				Payload: map[string]any{"name": name, "role": task.Role},
				Timestamp: time.Now().UTC(),
			})
		}
	}
	return s.complete(ctx, sessionSHA)
}

func (s *Service) runDiscovery(ctx context.Context, sessionSHA, input string, plan *ExecutionPlan) error {
	artifactMap := map[string][]string{
		"TechLead":         {"technical-design"},
		"SeniorQAEngineer": {"test-plan"},
		"APIArchitect":     {"api-spec"},
	}
	for _, task := range plan.Tasks {
		content, err := s.discovery.RunRole(ctx, sessionSHA, task.Role, input)
		if err != nil {
			return err
		}
		names := artifactMap[task.Role]
		if len(names) == 0 {
			names = []string{strings.ToLower(task.Role)}
		}
		for _, name := range names {
			if err := s.discovery.WriteArtifact(ctx, sessionSHA, name, task.Role, content); err != nil {
				return err
			}
			_ = s.events.Publish(ctx, event.Event{
				Type: event.ArtifactWritten, SessionSHA: sessionSHA,
				Payload: map[string]any{"name": name},
				Timestamp: time.Now().UTC(),
			})
		}
	}
	return s.complete(ctx, sessionSHA)
}

func (s *Service) runDelivery(ctx context.Context, sessionSHA, input string, plan *ExecutionPlan) error {
	var implSHAs []struct {
		sha, taskID string
	}
	for _, task := range plan.Tasks {
		switch task.Role {
		case "SeniorBackendTestDeveloper":
			_, err := s.delivery.RunTDDTests(ctx, sessionSHA, task.Role, input)
			if err != nil {
				return err
			}
		case "SeniorBackendDeveloper":
			sha, taskID, err := s.delivery.RunImplementation(ctx, sessionSHA, task.ID, task.Role, input, task.NeedsWorktree)
			if err != nil {
				_ = s.events.Publish(ctx, event.Event{
					Type: event.TaskFailed, SessionSHA: sessionSHA,
					Payload: map[string]any{"task_id": task.ID, "role": task.Role, "error": err.Error()},
					Timestamp: time.Now().UTC(),
				})
				return err
			}
			implSHAs = append(implSHAs, struct{ sha, taskID string }{sha, taskID})
		case "TechLead", "PeerReviewer", "SeniorQAEngineer":
			// reviews handled after implementation
		}
	}
	for _, impl := range implSHAs {
		body, _, err := s.store.GetBlob(ctx, impl.sha)
		if err != nil {
			return err
		}
		if err := s.delivery.TwoStageReview(ctx, sessionSHA, impl.sha, string(body)); err != nil {
			return err
		}
		_ = s.delivery.RemoveWorktree(ctx, sessionSHA, impl.taskID)
	}
	_ = s.delivery.PruneWorktrees(ctx, sessionSHA)
	return s.complete(ctx, sessionSHA)
}

func (s *Service) complete(ctx context.Context, sessionSHA string) error {
	_ = s.store.UpdateSessionState(ctx, sessionSHA, entity.StateCompleted)
	_ = s.otel.Flush(ctx)
	return s.events.Publish(ctx, event.Event{
		Type: event.SessionCompleted, SessionSHA: sessionSHA,
		Timestamp: time.Now().UTC(),
	})
}

func (s *Service) GetStatus(ctx context.Context, sessionSHA string) (entity.SessionState, error) {
	sess, err := s.store.GetSession(ctx, sessionSHA)
	if err != nil {
		return "", err
	}
	return sess.State, nil
}

func (s *Service) ResolveCheckpoint(ctx context.Context, sessionSHA, checkpointID, decision string) error {
	_ = sessionSHA
	return s.store.ResolveCheckpoint(ctx, checkpointID, entity.CheckpointDecision(decision))
}

// MetaAgentPlan uses the agent factory for dynamic planning extension point.
func (s *Service) MetaAgentPlan(ctx context.Context, sessionSHA, mode string) (*ExecutionPlan, error) {
	manifest, err := s.store.GetManifest(ctx, sessionSHA)
	if err != nil {
		return nil, err
	}
	ag, err := s.factory.NewWithSession(ctx, sessionSHA, agent.Input{
		Role: "MetaPlanner", SystemPrompt: "You are an orchestrator meta-agent.",
		UserPrompt: fmt.Sprintf("Plan execution for mode %s", mode), Provider: "claude",
	})
	if err != nil {
		return s.planner.Plan(ctx, mode, sessionSHA, manifest)
	}
	_, _ = ag.Run(ctx, agent.Input{Role: "MetaPlanner", UserPrompt: "plan", Provider: "claude"})
	return s.planner.Plan(ctx, mode, sessionSHA, manifest)
}
