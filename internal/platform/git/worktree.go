package git

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

type WorktreeOptions struct {
	ObjectiveID objective.ObjectiveID
	TaskID      string
	BaseBranch  string
	BranchName  string // computed: karakuri/<objective-id>/<task-id>
}

type Worktree struct {
	TaskID      string
	ObjectiveID objective.ObjectiveID
	Path        string
	Branch      string
	CreatedAt   time.Time
}

type WorktreeManager interface {
	Create(ctx context.Context, opts WorktreeOptions) (Worktree, error)
	Get(ctx context.Context, taskID string) (Worktree, error)
	Remove(ctx context.Context, taskID string) error
	List(ctx context.Context, objectiveID objective.ObjectiveID) ([]Worktree, error)
	Prune(ctx context.Context, objectiveID objective.ObjectiveID) error
}
