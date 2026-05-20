package cliagent

import (
	"context"
	"fmt"
	"strings"
)

// GeminiCLI is a CLIAgentAdapter that delegates tasks to the Gemini CLI
// (`@google/gemini-cli`). It invokes:
//
//	gemini --prompt "<prompt>"
//
// inside the requested worktree. Gemini CLI emits plain text on stdout
// (no NDJSON event stream); each line is forwarded as a text chunk.
type GeminiCLI struct {
	bin string // path to gemini binary; "gemini" by default
}

func NewGeminiCLI(bin string) *GeminiCLI {
	if bin == "" {
		bin = "gemini"
	}
	return &GeminiCLI{bin: bin}
}

func (g *GeminiCLI) Name() string { return "gemini_cli" }

func (g *GeminiCLI) Active() bool { return binaryAvailable(g.bin) }

func (g *GeminiCLI) Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error) {
	var out DelegateOutput
	var raw strings.Builder

	stream, err := g.Stream(ctx, in)
	if err != nil {
		return out, err
	}
	for chunk := range stream {
		raw.WriteString(chunk.Content)
		if chunk.Kind == "text" {
			out.Summary += chunk.Content
		}
		if chunk.Kind == "error" && chunk.Err != nil {
			return out, chunk.Err
		}
	}
	out.RawOutput = raw.String()
	return out, nil
}

func (g *GeminiCLI) Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error) {
	args := []string{"--prompt", in.Prompt}

	ch := make(chan DelegateChunk, 16)
	go func() {
		defer close(ch)

		exitCode, stderr, err := runStreaming(ctx, in, g.bin, args, func(line string) {
			ch <- DelegateChunk{Kind: "text", Content: line + "\n"}
		})

		if err != nil {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("gemini_cli: %w (stderr: %s)", err, stderr)}
			return
		}
		if exitCode != 0 {
			ch <- DelegateChunk{Kind: "error", Err: fmt.Errorf("gemini_cli: exit %d (stderr: %s)", exitCode, stderr)}
		}
		ch <- DelegateChunk{Kind: "done"}
	}()
	return ch, nil
}
