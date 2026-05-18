package git

import (
	"context"
	"time"
)

type WorktreeOptions struct {
	SessionSHA string
	TaskID     string
	BaseBranch string
	BranchName string
}

type Worktree struct {
	TaskID     string
	SessionSHA string
	Path       string
	Branch     string
	CreatedAt  time.Time
}

type WorktreeManager interface {
	Create(ctx context.Context, opts WorktreeOptions) (Worktree, error)
	Get(ctx context.Context, taskID string) (Worktree, error)
	Remove(ctx context.Context, taskID string) error
	List(ctx context.Context, sessionSHA string) ([]Worktree, error)
	Prune(ctx context.Context, sessionSHA string) error
}
