package cliagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CursorCLI is a CLIAgentAdapter that delegates tasks to the Cursor CLI agent
// (`cursor-agent`). It invokes:
//
//	cursor-agent --print --output-format=stream-json "<prompt>"
//
// inside the requested worktree. Per Cursor's documented stream-json reporter
// the output is NDJSON, with event types broadly similar to Claude Code's.
type CursorCLI struct {
	bin string // path to cursor-agent binary; "cursor-agent" by default
}

func NewCursorCLI(bin string) *CursorCLI {
	if bin == "" {
		bin = "cursor-agent"
	}
	return &CursorCLI{bin: bin}
}

func (c *CursorCLI) Name() string { return "cursor_cli" }

func (c *CursorCLI) Active() bool { return binaryAvailable(c.bin) }

func (c *CursorCLI) Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error) {
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

func (c *CursorCLI) Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error) {
	args := []string{"--print", "--output-format=stream-json"}
	if len(in.AllowedTools) > 0 {
		args = append(args, "--allowed-tools="+strings.Join(in.AllowedTools, ","))
	}
	args = append(args, in.Prompt)

	ch := make(chan DelegateChunk, 16)
	go func() {
		defer close(ch)

		exitCode, stderr, err := runStreaming(ctx, in, c.bin, args, func(line string) {
			parseCursorStreamLine(line, ch)
		})

		if err != nil {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("cursor_cli: %w (stderr: %s)", err, stderr)}
			return
		}
		if exitCode != 0 {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("cursor_cli: exit %d (stderr: %s)", exitCode, stderr)}
		}
		ch <- DelegateChunk{Kind: "done"}
	}()
	return ch, nil
}

// parseCursorStreamLine — Cursor CLI's stream-json output mirrors the
// Anthropic event shape (text + tool_use blocks). Unknown event types are
// surfaced as raw text so a format change degrades gracefully.
func parseCursorStreamLine(line string, ch chan<- DelegateChunk) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	// Cursor uses the same event taxonomy as Claude — reuse the same struct.
	var evt claudeEvent
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
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
		if evt.Result != "" {
			ch <- DelegateChunk{Kind: "text", Content: evt.Result}
		}
	}
}
