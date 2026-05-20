package cliagent

import (
	"context"
	"encoding/json"
	"fmt"
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
	args := []string{"--print", "--output-format=stream-json"}
	if len(in.AllowedTools) > 0 {
		args = append(args, "--allowed-tools="+strings.Join(in.AllowedTools, ","))
	}
	args = append(args, in.Prompt)

	ch := make(chan DelegateChunk, 16)
	go func() {
		defer close(ch)

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
	Type    string         `json:"type"`
	Message claudeMessage  `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
	Subtype string         `json:"subtype,omitempty"`
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
