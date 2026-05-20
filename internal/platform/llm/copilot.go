package llm

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
)

// CopilotProvider is intentionally a stub: GitHub Copilot does not expose a
// generally-available LLM API for individual subscribers. The supported
// integration path is the Copilot CLI agent (`gh copilot suggest/explain`)
// — see internal/platform/tools/cliagent/copilot.go and the cli_agents slot.
type CopilotProvider struct{}

func NewCopilotProvider() *CopilotProvider { return &CopilotProvider{} }

func (c *CopilotProvider) Name() string                     { return "copilot" }
func (c *CopilotProvider) Available(_ context.Context) bool { return false }
func (c *CopilotProvider) AsLLM() llms.Model                { return nil }

func (c *CopilotProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, fmt.Errorf("copilot API not supported; use the Copilot CLI agent (tools.cli_agents) instead")
}

func (c *CopilotProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan CompletionChunk, error) {
	return nil, fmt.Errorf("copilot API not supported; use the Copilot CLI agent (tools.cli_agents) instead")
}
