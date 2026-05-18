package llm

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type CursorProvider struct{}

func NewCursorProvider() *CursorProvider { return &CursorProvider{} }

func (c *CursorProvider) Name() string { return "cursor" }

func (c *CursorProvider) Available(_ context.Context) bool { return false }

func (c *CursorProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, errors.ErrNotImplemented
}

func (c *CursorProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan CompletionChunk, error) {
	return nil, errors.ErrNotImplemented
}
