package strategy

import (
	"context"
	"fmt"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
)

type Service struct {
	factory  *platformagent.Factory
	artifact *artifact.Service
}

func NewService(factory *platformagent.Factory, art *artifact.Service) *Service {
	return &Service{factory: factory, artifact: art}
}

var rolePrompts = map[string]string{
	"ProductManager":      "You are a Product Manager. Produce a concise PRD.",
	"Architect":           "You are a Software Architect. Produce a design document and ADR.",
	"EngineeringManager":  "You are an Engineering Manager. Produce a delivery roadmap.",
}

func (s *Service) RunRole(ctx context.Context, sessionSHA, role, userInput string) (string, error) {
	prompt, ok := rolePrompts[role]
	if !ok {
		prompt = fmt.Sprintf("You are a %s. Complete your assigned deliverable.", role)
	}
	ag, err := s.factory.NewWithSession(ctx, sessionSHA, agent.Input{
		Role: role, SystemPrompt: prompt,
		UserPrompt: userInput, Temperature: 0.3, Provider: "claude",
	})
	if err != nil {
		return "", err
	}
	out, err := ag.Run(ctx, agent.Input{
		Role: role, SystemPrompt: prompt,
		UserPrompt: userInput, Temperature: 0.3, Provider: "claude",
	})
	if err != nil {
		return "", err
	}
	return out.Content, nil
}

func (s *Service) WriteArtifact(ctx context.Context, sessionSHA, name, role, content string) error {
	_, err := s.artifact.Write(ctx, sessionSHA, name, role, []byte(content))
	return err
}
