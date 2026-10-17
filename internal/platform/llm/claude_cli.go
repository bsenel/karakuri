package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bsenel/karakuri/internal/platform/procenv"
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

// env is the environment the spawned CLI receives: the gateway attribution
// this deployment adds, minus the session markers it must not inherit.
//
// A method rather than an inline expression so the composition is testable
// without spawning the binary.
func (c *claudeCLI) env(ctx context.Context) []string {
	return procenv.ScrubNestedSession(gatewayEnv(ctx, os.Environ()))
}

func (c *claudeCLI) complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	prompt := req.SystemPrompt
	if prompt != "" {
		prompt += "\n\n"
	}
	prompt += lastUserMessage(req.Messages)

	cmd := exec.CommandContext(ctx, c.binary, "--print", "--output-format", "json")
	cmd.Stdin = strings.NewReader(prompt)
	// Per invocation, not per process: the CLI is spawned for each call, so
	// the customer header can vary by twin. Without this the CLI path would be
	// the one place a gateway could not attribute spend.
	cmd.Env = c.env(ctx)
	out, err := cmd.Output()
	if err != nil {
		// `cmd.Output()` populates *exec.ExitError.Stderr on non-zero exit;
		// surface it so callers see why the CLI rejected the request, and
		// name a rate limit there as one.
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return CompletionResponse{}, classifyRateLimit("claude",
				fmt.Errorf("claude CLI exited %w: %s", err, strings.TrimSpace(string(ee.Stderr))))
		}
		// A refused reply, the account's usage limit among them, comes back
		// as the JSON result on stdout with stderr empty; without reading it
		// every such exit read as a bare "exit status 1".
		if resp, perr := parseClaudeCLIResponse(out); perr == nil && resp.IsError {
			return CompletionResponse{}, classifyRateLimit("claude",
				fmt.Errorf("claude CLI exited %w: %s", err, resp.errorMessage()))
		}
		return CompletionResponse{}, fmt.Errorf("claude CLI exited: %w", err)
	}

	resp, err := parseClaudeCLIResponse(out)
	if err != nil {
		return CompletionResponse{}, err
	}
	if resp.IsError {
		return CompletionResponse{}, classifyRateLimit("claude", fmt.Errorf("claude CLI returned error: %s", resp.errorMessage()))
	}

	used := resp.Usage.InputTokens + resp.Usage.OutputTokens
	if used == 0 {
		used = len(resp.Result) / 4
	}
	return CompletionResponse{
		Content:      resp.Result,
		TokensUsed:   used,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, nil
}

// errorMessage is why the CLI marked its reply as failed. The CLI puts it in
// result; error is read first for older replies that set it.
func (r claudeCLIResponse) errorMessage() string {
	if r.Error != "" {
		return r.Error
	}
	if msg := strings.TrimSpace(r.Result); msg != "" {
		return msg
	}
	return "unknown error"
}

// parseClaudeCLIResponse reads the CLI's JSON reply from out. The CLI
// sometimes emits a banner on first run, so the last JSON line is tried
// before giving up.
func parseClaudeCLIResponse(out []byte) (claudeCLIResponse, error) {
	var resp claudeCLIResponse
	jerr := json.Unmarshal(out, &resp)
	if jerr == nil {
		return resp, nil
	}
	if line := lastJSONLine(out); line != "" {
		if json.Unmarshal([]byte(line), &resp) == nil {
			return resp, nil
		}
	}
	return claudeCLIResponse{}, fmt.Errorf("claude CLI: parse JSON: %w (raw: %.200s)", jerr, string(out))
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
