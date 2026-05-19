package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type GeminiProvider struct{}

func NewGeminiProvider() *GeminiProvider { return &GeminiProvider{} }

func (g *GeminiProvider) Name() string                        { return "gemini" }
func (g *GeminiProvider) Available(_ context.Context) bool    { return false }
func (g *GeminiProvider) AsLLM() llms.Model                   { return nil }

func (g *GeminiProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, errors.ErrNotImplemented
}

func (g *GeminiProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan CompletionChunk, error) {
	return nil, errors.ErrNotImplemented
}
