package delivery

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Service struct {
	factory   *platformagent.Factory
	artifact  *artifact.Service
	worktrees git.WorktreeManager
	store     storage.StorageAdapter
	reviewer  *Reviewer
	events    *event.Hub
	otel      *observability.OTel
}

func NewService(
	factory *platformagent.Factory,
	art *artifact.Service,
	wt git.WorktreeManager,
	store storage.StorageAdapter,
	reviewer *Reviewer,
	events *event.Hub,
	otel *observability.OTel,
) *Service {
	return &Service{factory: factory, artifact: art, worktrees: wt, store: store, reviewer: reviewer, events: events, otel: otel}
}

func (s *Service) RunImplementation(ctx context.Context, sessionSHA, taskID, role, userInput string, needsWorktree bool) (string, string, error) {
	workDir := ""
	if needsWorktree {
		wt, err := s.worktrees.Create(ctx, git.WorktreeOptions{
			SessionSHA: sessionSHA, TaskID: taskID,
		})
		if err != nil {
			return "", "", err
		}
		_ = s.store.SaveWorktree(ctx, wt)
		s.otel.IncWorktreeCreated()
		_ = s.events.Publish(ctx, event.Event{
			Type: event.WorktreeCreated, SessionSHA: sessionSHA,
			Payload: map[string]any{"task_id": taskID, "path": wt.Path, "branch": wt.Branch},
			Timestamp: time.Now().UTC(),
		})
		workDir = wt.Path
	}
	prompt := fmt.Sprintf("You are a %s. Implement the task using TDD. Write production-ready code.", role)
	ag, err := s.factory.NewWithSession(ctx, sessionSHA, agent.Input{Role: role, Provider: "claude"})
	if err != nil {
		return "", "", err
	}
	out, err := ag.Run(ctx, agent.Input{
		Role: role, SystemPrompt: prompt, UserPrompt: userInput,
		Temperature: 0.3, Provider: "claude", WorkDir: workDir,
	})
	if err != nil {
		return "", "", err
	}
	artName := fmt.Sprintf("implementation-%s", taskID)
	art, err := s.artifact.Write(ctx, sessionSHA, artName, role, []byte(out.Content))
	if err != nil {
		return "", "", err
	}
	return art.SHA, taskID, nil
}

func (s *Service) RunTDDTests(ctx context.Context, sessionSHA, role, userInput string) (string, error) {
	prompt := "You are a Senior Backend Test Developer. Write failing tests first (TDD red phase)."
	ag, err := s.factory.NewWithSession(ctx, sessionSHA, agent.Input{Role: role, Provider: "claude"})
	if err != nil {
		return "", err
	}
	out, err := ag.Run(ctx, agent.Input{
		Role: role, SystemPrompt: prompt, UserPrompt: userInput, Temperature: 0.2, Provider: "claude",
	})
	if err != nil {
		return "", err
	}
	art, err := s.artifact.Write(ctx, sessionSHA, "backend-tests", role, []byte(out.Content))
	return art.SHA, err
}

func (s *Service) TwoStageReview(ctx context.Context, sessionSHA, artifactSHA, implContent string) error {
	if _, err := s.reviewer.RunReview(ctx, sessionSHA, "TechLead", artifactSHA, implContent); err != nil {
		return err
	}
	_, err := s.reviewer.RunReview(ctx, sessionSHA, "PeerReviewer", artifactSHA, implContent)
	return err
}

func (s *Service) RemoveWorktree(ctx context.Context, sessionSHA, taskID string) error {
	if err := s.worktrees.Remove(ctx, taskID); err != nil {
		return err
	}
	_ = s.store.DeleteWorktree(ctx, taskID)
	s.otel.IncWorktreeRemoved()
	return s.events.Publish(ctx, event.Event{
		Type: event.WorktreeRemoved, SessionSHA: sessionSHA,
		Payload: map[string]any{"task_id": taskID},
		Timestamp: time.Now().UTC(),
	})
}

func (s *Service) PruneWorktrees(ctx context.Context, sessionSHA string) error {
	wts, _ := s.worktrees.List(ctx, sessionSHA)
	count := len(wts)
	if err := s.worktrees.Prune(ctx, sessionSHA); err != nil {
		return err
	}
	return s.events.Publish(ctx, event.Event{
		Type: event.WorktreePruned, SessionSHA: sessionSHA,
		Payload: map[string]any{"session_sha": sessionSHA, "count": count},
		Timestamp: time.Now().UTC(),
	})
}

func (s *Service) ListWorktrees(ctx context.Context, sessionSHA string) ([]git.Worktree, error) {
	return s.store.ListWorktrees(ctx, sessionSHA)
}
