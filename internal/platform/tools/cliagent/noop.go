package cliagent

import (
	"context"
	"log/slog"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Name() string { return "noop" }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error) {
	slog.WarnContext(ctx, "CLIAgentAdapter not configured: delegation skipped",
		"prompt_len", len(in.Prompt), "worktree", in.WorktreePath)
	return DelegateOutput{Summary: "no-op: no CLI agent configured"}, nil
}

func (n *NoOp) Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error) {
	ch := make(chan DelegateChunk, 1)
	go func() {
		defer close(ch)
		out, _ := n.Delegate(ctx, in)
		ch <- DelegateChunk{Kind: "text", Content: out.Summary}
		ch <- DelegateChunk{Kind: "done"}
	}()
	return ch, nil
}
