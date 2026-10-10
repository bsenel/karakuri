package cliagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeCode is a CLIAgentAdapter that delegates tasks to the Claude Code CLI.
// It invokes:
//
//	claude --print --output-format=stream-json "<prompt>"
//
// inside the requested worktree. The stream-json reporter emits NDJSON events;
// we parse each line into a DelegateChunk so the loop can stream live output
// AND we accumulate a final DelegateOutput at completion.
type ClaudeCode struct {
	bin string // path to claude binary; "claude" by default
}

func NewClaudeCode(bin string) *ClaudeCode {
	if bin == "" {
		bin = "claude"
	}
	return &ClaudeCode{bin: bin}
}

func (c *ClaudeCode) Name() string { return "claude_code" }

func (c *ClaudeCode) Active() bool { return binaryAvailable(c.bin) }

func (c *ClaudeCode) Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error) {
	var out DelegateOutput
	var raw strings.Builder

	stream, err := c.Stream(ctx, in)
	if err != nil {
		return out, err
	}
	for chunk := range stream {
		raw.WriteString(chunk.Content)
		switch chunk.Kind {
		case "text":
			out.Summary += chunk.Content
		case "tool_use":
			if chunk.Tool != nil {
				out.ToolUses = append(out.ToolUses, *chunk.Tool)
			}
		case "error":
			if chunk.Err != nil {
				return out, chunk.Err
			}
		}
	}
	out.RawOutput = raw.String()
	return out, nil
}

func (c *ClaudeCode) Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error) {
	if in.MCP != nil && len(in.MCP.Tools) == 0 {
		return nil, fmt.Errorf("claude_code: MCP server %q attached with no tool: nothing would be allowed", in.MCP.ServerName)
	}

	allowed := in.AllowedTools
	if in.MCP != nil {
		allowed = append([]string{}, in.AllowedTools...)
		for _, tool := range in.MCP.Tools {
			allowed = append(allowed, "mcp__"+in.MCP.ServerName+"__"+tool)
		}
	}
	args := []string{"--print", "--output-format=stream-json", "--verbose"}
	if len(allowed) > 0 {
		args = append(args, "--allowed-tools="+strings.Join(allowed, ","))
	}

	ch := make(chan DelegateChunk, 16)
	go func() {
		defer close(ch)

		if in.MCP != nil {
			dir, path, err := writeClaudeMCPConfig(in.MCP)
			if dir != "" {
				// The file holds the run's credential: it goes when the run does, on every path.
				defer func() { _ = os.RemoveAll(dir) }()
			}
			if err != nil {
				ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("claude_code: %w", err)}
				return
			}
			args = append(args, "--mcp-config", path, "--strict-mcp-config")
		}
		args = append(args, in.Prompt)

		exitCode, stderr, err := runStreaming(ctx, in, c.bin, args, func(line string) {
			parseClaudeStreamLine(line, ch)
		})

		if err != nil {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("claude_code: %w (stderr: %s)", err, stderr)}
			return
		}
		if exitCode != 0 {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("claude_code: exit %d (stderr: %s)", exitCode, stderr)}
		}
		ch <- DelegateChunk{Kind: "done"}
	}()
	return ch, nil
}

// writeClaudeMCPConfig writes the one-server MCP configuration for one run
// into a fresh directory under os.TempDir() (0700, file 0600), so the token
// reaches the CLI through neither argv nor the environment and never lands in
// the worktree. The caller removes dir whenever it is non-empty.
//
// NOT VERIFIED AGAINST THE CLI: the JSON shape of --mcp-config
// ({"mcpServers": {<name>: {"type": "http", "url", "headers"}}}), the
// mcp__<server>__<tool> naming used in the allow-list, and the effect of
// --strict-mcp-config (ignore every other MCP configuration) were not read
// from `claude --help` and were not run against a real claude binary. They
// come from the planner's recollection of Claude Code's documentation. The
// scripted-binary tests prove what Karakuri passes, not that the claude binary
// accepts it. Which MCP protocol revisions the CLI speaks is also unknown.
func writeClaudeMCPConfig(m *MCPAttachment) (dir, path string, err error) {
	type server struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	body, err := json.Marshal(map[string]map[string]server{
		"mcpServers": {m.ServerName: {
			Type:    "http",
			URL:     m.URL,
			Headers: map[string]string{"Authorization": "Bearer " + m.Token},
		}},
	})
	if err != nil {
		return "", "", fmt.Errorf("encode MCP configuration: %w", err)
	}
	dir, err = os.MkdirTemp("", "karakuri-mcp-")
	if err != nil {
		return "", "", fmt.Errorf("create MCP configuration directory: %w", err)
	}
	path = filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return dir, "", fmt.Errorf("write MCP configuration: %w", err)
	}
	return dir, path, nil
}

// parseClaudeStreamLine inspects one NDJSON line from `claude --output-format=stream-json`
// and emits one or more DelegateChunks. Claude Code's event shape (paraphrased):
//
//	{"type": "system", "subtype": "init", ...}
//	{"type": "assistant", "message": {"content": [
//	    {"type": "text", "text": "..."},
//	    {"type": "tool_use", "id": "...", "name": "...", "input": {...}}
//	]}}
//	{"type": "user", "message": {"content": [
//	    {"type": "tool_result", "tool_use_id": "...", "content": "..."}
//	]}}
//	{"type": "result", "result": "...", "subtype": "success"}
//
// Unknown shapes are silently swallowed so a CLI format change degrades the
// adapter rather than crashing the loop.
func parseClaudeStreamLine(line string, ch chan<- DelegateChunk) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var evt claudeEvent
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
		// Not JSON — surface raw text so callers can still see CLI output.
		ch <- DelegateChunk{Kind: "text", Content: line + "\n"}
		return
	}
	switch evt.Type {
	case "assistant":
		for _, block := range evt.Message.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					ch <- DelegateChunk{Kind: "text", Content: block.Text}
				}
			case "tool_use":
				ch <- DelegateChunk{Kind: "tool_use", Tool: &ToolUse{
					Name: block.Name, Input: block.Input, OK: true,
				}}
			}
		}
	case "user":
		// tool_result blocks live inside user messages; surface them so callers
		// know whether a tool succeeded.
		for _, block := range evt.Message.Content {
			if block.Type == "tool_result" {
				ch <- DelegateChunk{Kind: "tool_result", Tool: &ToolUse{
					Name:   block.ToolUseID,
					Result: stringifyContent(block.Content),
					OK:     !block.IsError,
				}}
			}
		}
	case "result":
		// Final summary line — already accumulated via "assistant" text blocks,
		// but emit explicitly so callers don't depend on internal accumulation.
		if evt.Result != "" {
			ch <- DelegateChunk{Kind: "text", Content: evt.Result}
		}
	}
}

type claudeEvent struct {
	Type    string        `json:"type"`
	Message claudeMessage `json:"message,omitempty"`
	Result  string        `json:"result,omitempty"`
	Subtype string        `json:"subtype,omitempty"`
}

type claudeMessage struct {
	Content []claudeBlock `json:"content"`
}

type claudeBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"` // tool_result can be string or []block
	IsError   bool           `json:"is_error,omitempty"`
}

func stringifyContent(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	// Could be array of blocks for nested content; serialize defensively.
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return ""
}
