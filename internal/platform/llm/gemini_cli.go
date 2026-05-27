package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// geminiCLI shells out to the `gemini` CLI as a transparent fallback when
// GOOGLE_API_KEY / GOOGLE_AI_API_KEY are unset. The CLI handles its own
// auth (Google Cloud application-default credentials, gcloud login, or
// vendor-specific env vars).
//
// Invocation: `gemini --prompt <text>`. Output is plain text — there's no
// official "single-shot JSON" mode (`gcloud gemini-cli` is agentic; the
// open-source `@google/gemini-cli` is also agent-oriented). We accept the
// degraded path: TokensUsed is reported as 0, callers that depend on token
// counts should prefer the API path (set GOOGLE_API_KEY).
type geminiCLI struct {
	binary string // path to the gemini binary; resolved at construction
}

func newGeminiCLI(binary string) *geminiCLI {
	return &geminiCLI{binary: binary}
}

func (g *geminiCLI) complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	prompt := req.SystemPrompt
	if prompt != "" {
		prompt += "\n\n"
	}
	prompt += lastUserMessage(req.Messages)

	cmd := exec.CommandContext(ctx, g.binary, "--prompt", prompt)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return CompletionResponse{}, fmt.Errorf("gemini CLI exited %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return CompletionResponse{}, fmt.Errorf("gemini CLI exited: %w", err)
	}
	content := strings.TrimRight(string(out), "\n")
	// TokensUsed=0: no usage metadata available from plain-text output.
	return CompletionResponse{Content: content, TokensUsed: 0}, nil
}
