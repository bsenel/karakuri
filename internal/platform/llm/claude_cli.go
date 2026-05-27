package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// claudeCLI shells out to the `claude` CLI as a transparent fallback when
// ANTHROPIC_API_KEY is unset. The CLI handles its own auth (OAuth via
// `claude /login`, env-var API key, or apiKeyHelper) so we don't read the
// credentials file directly.
//
// Invocation: `claude --print --output-format json` with the assembled
// prompt on stdin. Response shape: {session_id, result, usage:{input_tokens,
// output_tokens}}. Token counts come from the CLI directly when present;
// fall back to a 4-chars-per-token estimate otherwise (matches the API
// path's behavior in claude.go).
type claudeCLI struct {
	binary string // path to the claude binary; resolved at construction
}

func newClaudeCLI(binary string) *claudeCLI {
	return &claudeCLI{binary: binary}
}

// claudeCLIResponse mirrors the JSON shape emitted by `claude --print
// --output-format json`. Extra fields are ignored.
type claudeCLIResponse struct {
	Result string `json:"result"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	IsError bool   `json:"is_error,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (c *claudeCLI) complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	prompt := req.SystemPrompt
	if prompt != "" {
		prompt += "\n\n"
	}
	prompt += lastUserMessage(req.Messages)

	cmd := exec.CommandContext(ctx, c.binary, "--print", "--output-format", "json")
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		// `cmd.Output()` populates *exec.ExitError.Stderr on non-zero exit;
		// surface it so callers see why the CLI rejected the request.
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return CompletionResponse{}, fmt.Errorf("claude CLI exited %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return CompletionResponse{}, fmt.Errorf("claude CLI exited: %w", err)
	}

	var resp claudeCLIResponse
	if jerr := json.Unmarshal(out, &resp); jerr != nil {
		// The CLI sometimes emits a banner on first run; try parsing only
		// the last line as JSON before giving up.
		if line := lastJSONLine(out); line != "" {
			if jerr2 := json.Unmarshal([]byte(line), &resp); jerr2 == nil {
				goto parsed
			}
		}
		return CompletionResponse{}, fmt.Errorf("claude CLI: parse JSON: %w (raw: %.200s)", jerr, string(out))
	}
parsed:
	if resp.IsError {
		msg := resp.Error
		if msg == "" {
			msg = "unknown error"
		}
		return CompletionResponse{}, fmt.Errorf("claude CLI returned error: %s", msg)
	}

	used := resp.Usage.InputTokens + resp.Usage.OutputTokens
	if used == 0 {
		used = len(resp.Result) / 4
	}
	return CompletionResponse{Content: resp.Result, TokensUsed: used}, nil
}

// lastJSONLine returns the final non-empty line of out, trimmed. Used to
// recover from CLI banners or progress text printed before the JSON
// response on first invocation.
func lastJSONLine(out []byte) string {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" && strings.HasPrefix(s, "{") {
			return s
		}
	}
	return ""
}
