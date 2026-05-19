package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type CopilotProvider struct{}

func NewCopilotProvider() *CopilotProvider { return &CopilotProvider{} }

func (c *CopilotProvider) Name() string                        { return "copilot" }
func (c *CopilotProvider) Available(_ context.Context) bool    { return false }
func (c *CopilotProvider) AsLLM() llms.Model                   { return nil }

func (c *CopilotProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, errors.ErrNotImplemented
}

func (c *CopilotProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan CompletionChunk, error) {
	return nil, errors.ErrNotImplemented
}
