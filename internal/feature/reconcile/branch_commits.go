package reconcile

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// BranchCommitsFunc reads a stream's branch and the commits on it ahead of its
// base (Phase 40 step 9a); it is the shape Service.branchCommits has.
type BranchCommitsFunc func(ctx context.Context, obj objective.Objective) (branch string, commits []string, err error)

// NewBranchCommitsReader is the production reader of a cut-off pass's branch:
// the branch is named by the write actions the cut-off loop planned
// (params.branch, base params.base_branch or main), and its commits come from
// the version control instance bound to the objective's twin.
//
// Scaffold: the implementation is the next slice's.
func NewBranchCommitsReader(
	store storage.StorageAdapter,
	vc func(ctx context.Context, obj objective.Objective) versioncontrol.VersionControlAdapter,
) BranchCommitsFunc {
	return func(context.Context, objective.Objective) (string, []string, error) {
		return "", nil, errors.New("branch commits reader not implemented")
	}
}

// WithBranchCommits sets what reads the stream's branch for a resumed pass.
// Nil leaves resumed passes saying their commits are unreadable.
func (s *Service) WithBranchCommits(read BranchCommitsFunc) *Service {
	s.branchCommits = read
	return s
}
