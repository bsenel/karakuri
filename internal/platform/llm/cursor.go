package llm

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
)

// CursorProvider is intentionally a stub: Cursor does not offer a generally-
// available LLM API for individual subscribers. The supported integration
// path is the Cursor CLI agent — see internal/platform/tools/cliagent/cursor.go
// and the cli_agents slot in config.Tools.
type CursorProvider struct{}

func NewCursorProvider() *CursorProvider { return &CursorProvider{} }

func (c *CursorProvider) Name() string                     { return "cursor" }
func (c *CursorProvider) Available(_ context.Context) bool { return false }
func (c *CursorProvider) AsLLM() llms.Model                { return nil }

func (c *CursorProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, fmt.Errorf("cursor API not supported; use the Cursor CLI agent (tools.cli_agents) instead")
}

func (c *CursorProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan CompletionChunk, error) {
	return nil, fmt.Errorf("cursor API not supported; use the Cursor CLI agent (tools.cli_agents) instead")
}
