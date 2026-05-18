package research

import (
	"context"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/session"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Service struct {
	tools    *tools.Registry
	artifact *artifact.Service
	sessions *session.Service
	store    storage.StorageAdapter
}

func NewService(t *tools.Registry, art *artifact.Service, sess *session.Service, store storage.StorageAdapter) *Service {
	return &Service{tools: t, artifact: art, sessions: sess, store: store}
}

type Request struct {
	Topic   string
	Sources []string
	Depth   string
}

func (s *Service) Run(ctx context.Context, req Request) (entity.Session, error) {
	sess, err := s.sessions.Create(ctx, session.CreateRequest{
		Mode: entity.ModeAutonomous, Input: req.Topic,
	})
	if err != nil {
		return entity.Session{}, err
	}
	srcs := req.Sources
	if len(srcs) == 0 {
		srcs = []string{"http-scraper"}
	}
	findings, err := s.tools.Research.Search(ctx, req.Topic, srcs, req.Depth)
	if err != nil {
		return entity.Session{}, err
	}
	var summary strings.Builder
	var confidence float64
	for _, f := range findings {
		summary.WriteString(f.Summary)
		summary.WriteString("\n")
		confidence = f.Confidence
	}
	art, _ := s.artifact.Write(ctx, sess.SHA, "research-result", "research", []byte(summary.String()))
	_ = s.store.SaveResearchResult(ctx, entity.ResearchResult{
		SHA: art.SHA, SessionSHA: sess.SHA, Topic: req.Topic,
		Summary: summary.String(), Confidence: confidence, Sources: srcs,
		CreatedAt: time.Now().UTC(),
	})
	return sess, nil
}
