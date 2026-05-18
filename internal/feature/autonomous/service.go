package autonomous

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/orchestrator"
	"github.com/bsenel/karakuri/internal/feature/session"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Service struct {
	tools    *tools.Registry
	artifact *artifact.Service
	sessions *session.Service
	events   *event.Hub
	workflowsDir string
}

func NewService(t *tools.Registry, art *artifact.Service, sess *session.Service, events *event.Hub, workflowsDir string) *Service {
	return &Service{tools: t, artifact: art, sessions: sess, events: events, workflowsDir: workflowsDir}
}

func (s *Service) RunCycle(ctx context.Context, sessionSHA string) error {
	wf, err := orchestrator.LoadWorkflow(s.workflowsDir, "autonomous")
	if err != nil {
		return err
	}
	for _, loop := range wf.Loops {
		if err := s.runLoop(ctx, sessionSHA, loop.Name, loop.Adapter); err != nil {
			continue
		}
	}
	return nil
}

func (s *Service) runLoop(ctx context.Context, sessionSHA, name, adapter string) error {
	active := s.isAdapterActive(adapter)
	if !active {
		_, _ = s.artifact.Write(ctx, sessionSHA, name, "autonomous", []byte("skipped"))
		return s.events.Publish(ctx, event.Event{
			Type: event.AdapterSkipped, SessionSHA: sessionSHA,
			Payload: map[string]any{"adapter": adapter, "loop": name, "reason": "not configured"},
			Timestamp: time.Now().UTC(),
		})
	}
	var content string
	switch name {
	case "commit-digest":
		commits, _ := s.tools.VC.GetCommits(ctx, "", time.Now().Add(-24*time.Hour))
		content = fmt.Sprintf("commit-digest: %d commits", len(commits))
	case "pr-review":
		prs, _ := s.tools.VC.ListPRs(ctx, "", time.Now().Add(-24*time.Hour))
		content = fmt.Sprintf("pr-review: %d prs", len(prs))
	case "slack-digest":
		msgs, _ := s.tools.Messaging.GetMessages(ctx, "", time.Now().Add(-24*time.Hour))
		content = fmt.Sprintf("slack-digest: %d messages", len(msgs))
	case "env-audit":
		alerts, _ := s.tools.Observability.GetAlerts(ctx, "prod", "", time.Now().Add(-24*time.Hour), "high")
		content = fmt.Sprintf("env-audit: %d alerts", len(alerts))
	case "research-pulse":
		findings, _ := s.tools.Research.Search(ctx, "industry trends", nil, "standard")
		if len(findings) > 0 {
			content = findings[0].Summary
			if findings[0].Confidence >= 0.75 {
				_ = s.events.Publish(ctx, event.Event{
					Type: event.PromotionReady, SessionSHA: sessionSHA,
					Payload: map[string]any{"artifact": name, "suggested_via": "strategy"},
					Timestamp: time.Now().UTC(),
				})
			}
		}
	default:
		content = name
	}
	_, err := s.artifact.Write(ctx, sessionSHA, name, "autonomous", []byte(content))
	return err
}

func (s *Service) isAdapterActive(adapter string) bool {
	switch adapter {
	case "versioncontrol":
		return s.tools.VC.Active()
	case "messaging":
		return s.tools.Messaging.Active()
	case "observability":
		return s.tools.Observability.Active()
	case "research":
		return s.tools.Research.Active()
	default:
		return false
	}
}

func (s *Service) Promote(ctx context.Context, fromSHA string, via entity.SessionMode, dryRun bool) (entity.Session, error) {
	if dryRun {
		return entity.Session{SHA: "dry-run", Mode: via}, nil
	}
	sess, err := s.sessions.Create(ctx, session.CreateRequest{Mode: via, ParentSHA: fromSHA, Input: "promoted from autonomous"})
	if err != nil {
		return entity.Session{}, err
	}
	return sess, nil
}
