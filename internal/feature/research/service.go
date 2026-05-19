package research

import (
	"context"
	"strings"

	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Service struct {
	tools    *tools.Registry
	artifact *artifact.Service
}

func NewService(t *tools.Registry, art *artifact.Service) *Service {
	return &Service{tools: t, artifact: art}
}

type Request struct {
	TwinID      string
	ObjectiveID string
	AgentID     string
	Topic       string
	Sources     []string
	Depth       string
}

type Result struct {
	ArtifactSHA string
	Summary     string
	Confidence  float64
}

func (s *Service) Run(ctx context.Context, req Request) (Result, error) {
	srcs := req.Sources
	if len(srcs) == 0 {
		srcs = []string{"http-scraper"}
	}
	findings, err := s.tools.Research.Search(ctx, req.Topic, srcs, req.Depth)
	if err != nil {
		return Result{}, err
	}
	var sb strings.Builder
	var confidence float64
	for _, f := range findings {
		sb.WriteString(f.Summary)
		sb.WriteString("\n")
		confidence = f.Confidence
	}
	art, err := s.artifact.Write(ctx, req.ObjectiveID, req.AgentID, "software.reason.research", []byte(sb.String()))
	if err != nil {
		return Result{}, err
	}
	return Result{ArtifactSHA: art.SHA, Summary: sb.String(), Confidence: confidence}, nil
}
