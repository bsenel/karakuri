package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
// the version control instance bound to the objective's twin. It never returns
// an empty list for a branch it could not read: no branch named, no adapter or
// an adapter error is an error.
func NewBranchCommitsReader(
	store storage.StorageAdapter,
	vc func(ctx context.Context, obj objective.Objective) versioncontrol.VersionControlAdapter,
) BranchCommitsFunc {
	return func(ctx context.Context, obj objective.Objective) (string, []string, error) {
		its, err := store.ListLoopIterations(ctx, obj.ID)
		if err != nil {
			return "", nil, fmt.Errorf("list loop iterations: %w", err)
		}
		cut := cutLoopFrom(ctx)
		repo, base, branch, ok := plannedBranch(its, cut)
		if !ok {
			if cut != nil {
				return "", nil, errors.New("the branch is unknown: the cut-off loop saved no plan that names one")
			}
			return "", nil, errors.New("no planned write action names a branch")
		}
		adapter := vc(ctx, obj)
		if adapter == nil {
			return branch, nil, errors.New("no version control adapter is bound")
		}
		found, err := adapter.CompareCommits(ctx, repo, base, branch)
		if err != nil {
			return branch, nil, fmt.Errorf("compare %s...%s: %w", base, branch, err)
		}
		shas := make([]string, len(found))
		for i, c := range found {
			shas[i] = c.SHA
		}
		return branch, shas, nil
	}
}

type cutLoopKey struct{}

// withCutLoop tells the reader which loop was cut off, so it reads that loop's
// own plan (the "act" row the loop saves before acting) and never another
// pass's. An empty id is a cut-off pass that recorded no loop.
func withCutLoop(ctx context.Context, loopID string) context.Context {
	return context.WithValue(ctx, cutLoopKey{}, &loopID)
}

func cutLoopFrom(ctx context.Context) *string {
	id, _ := ctx.Value(cutLoopKey{}).(*string)
	return id
}

// plannedBranch finds the branch of the most recent planned write action that
// names one. Reads (verify, observe) may name other branches and are skipped.
// With a cut-off loop it reads only that loop's "act" rows; without one, the
// newest "learn" row of the objective.
func plannedBranch(its []storage.LoopIteration, cut *string) (repo, base, branch string, ok bool) {
	for i := len(its) - 1; i >= 0; i-- {
		if cut != nil {
			if its[i].Step != "act" || *cut == "" {
				continue
			}
		} else if its[i].Step != "learn" {
			continue
		}
		var in struct {
			LoopID string `json:"loop_id"`
			Plan   struct {
				Actions []struct {
					Capability string         `json:"capability"`
					Params     map[string]any `json:"params"`
				} `json:"actions"`
			} `json:"plan"`
		}
		if json.Unmarshal([]byte(its[i].InputJSON), &in) != nil {
			continue
		}
		if cut != nil && in.LoopID != *cut {
			continue
		}
		for j := len(in.Plan.Actions) - 1; j >= 0; j-- {
			a := in.Plan.Actions[j]
			if !strings.Contains(a.Capability, ".act.") {
				continue
			}
			b, _ := a.Params["branch"].(string)
			if b == "" {
				continue
			}
			base, _ = a.Params["base_branch"].(string)
			if base == "" {
				base = "main"
			}
			repo, _ = a.Params["repo"].(string)
			return repo, base, b, true
		}
	}
	return "", "", "", false
}

// WithBranchCommits sets what reads the stream's branch for a resumed pass.
// Nil leaves resumed passes saying their commits are unreadable.
func (s *Service) WithBranchCommits(read BranchCommitsFunc) *Service {
	s.branchCommits = read
	return s
}
