package versioncontrol

import (
	"context"
	"log/slog"
	"time"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) CreatePR(ctx context.Context, pr PullRequest) (string, error) {
	slog.WarnContext(ctx, "VersionControlAdapter not configured: PR creation skipped", "title", pr.Title)
	return "", nil
}

func (n *NoOp) ListPRs(_ context.Context, _ string, _ time.Time) ([]PRSummary, error) {
	return nil, nil
}

func (n *NoOp) GetCommits(_ context.Context, _ string, _ time.Time) ([]Commit, error) {
	return nil, nil
}
