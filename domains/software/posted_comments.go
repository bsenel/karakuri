package software

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

const (
	postedIDsKey    = "posted_comment_ids"
	postedIDsErrKey = "posted_comment_ids_error"
)

// postedCommentCapabilities are the delegations that can post a comment, and
// so the only audit rows that carry posted_comment_ids.
var postedCommentCapabilities = []string{
	"software.act.write_code",
	"software.act.write_test",
	"software.act.delegate_to_cli",
}

// newPostedCommentReader reads, from the audit log, the IDs of the comments
// Karakuri is recorded as having posted (Phase 40 step 12a). It reads only the
// git observation window, the same reach as the comments it is asked about. A
// row that says the run's comments could not be read fails the read: a comment
// from a shared account would otherwise pass for the owner's.
func newPostedCommentReader(s storage.StorageAdapter) postedCommentIDs {
	return func(ctx context.Context) (map[int64]bool, error) {
		since := time.Now().UTC().Add(-gitObservationWindow)
		events, err := s.ListToolEvents(ctx, storage.ToolEventFilter{
			Kind:           storage.ToolEventExecute,
			CreatedAtSince: &since,
			OldestFirst:    true,
		})
		if err != nil {
			return nil, fmt.Errorf("list tool events: %w", err)
		}
		ids := map[int64]bool{}
		for _, ev := range events {
			if !isPostedCommentCapability(ev.Capability) {
				continue
			}
			var payload struct {
				Result environment.ActionResult `json:"result"`
			}
			if err := json.Unmarshal([]byte(ev.PayloadJSON), &payload); err != nil {
				return nil, fmt.Errorf("tool event %s has an unreadable payload: %w", ev.ID, err)
			}
			delta := payload.Result.StateDelta
			if msg, bad := delta[postedIDsErrKey]; bad {
				return nil, fmt.Errorf("tool event %s could not record its posted comments: %v", ev.ID, msg)
			}
			list, _ := delta[postedIDsKey].([]any)
			for _, x := range list {
				if n, ok := x.(float64); ok {
					ids[int64(n)] = true
				}
			}
		}
		return ids, nil
	}
}

func isPostedCommentCapability(c string) bool {
	for _, k := range postedCommentCapabilities {
		if c == k {
			return true
		}
	}
	return false
}

// commentRecorder lists a branch's pull request comments before a delegation
// and, after it, says which new ones Karakuri authored (Phase 40 step 12a).
type commentRecorder struct {
	vc         versioncontrol.VersionControlAdapter
	repo       string
	branch     string
	authors    config.OwnAuthorsConfig
	before     map[int64]bool
	beforeErr  error
	recordable bool
}

// startCommentRecorder reads the branch's comments; it records nothing when no
// forge is bound or active or the action names no branch.
func startCommentRecorder(ctx context.Context, vc versioncontrol.VersionControlAdapter, authors config.OwnAuthorsConfig, repo, branch string) *commentRecorder {
	r := &commentRecorder{vc: vc, repo: repo, branch: branch, authors: authors}
	if vc == nil || !vc.Active() || branch == "" {
		return r
	}
	r.recordable = true
	comments, _, err := branchComments(ctx, vc, repo, branch)
	if err != nil {
		r.beforeErr = err
		return r
	}
	r.before = make(map[int64]bool, len(comments))
	for _, c := range comments {
		r.before[c.ID] = true
	}
	return r
}

// finish returns the keys a delegation's result carries: posted_comment_ids
// when both reads worked, posted_comment_ids_error when either did not, and
// nothing when there was nothing to read. Never an empty list for a failure.
func (r *commentRecorder) finish(ctx context.Context) map[string]any {
	if !r.recordable {
		return nil
	}
	fail := func(err error) map[string]any {
		return map[string]any{postedIDsErrKey: err.Error()}
	}
	if r.beforeErr != nil {
		return fail(r.beforeErr)
	}
	// The run's own context may be the reason it ended.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	defer cancel()
	after, found, err := branchComments(rctx, r.vc, r.repo, r.branch)
	if err != nil {
		return fail(err)
	}
	if !found && len(r.before) == 0 {
		return nil
	}
	ids := []int64{}
	for _, c := range after {
		if !r.before[c.ID] && loginIn(r.authors.Karakuri, c.Author) {
			ids = append(ids, c.ID)
		}
	}
	return map[string]any{postedIDsKey: ids}
}

// branchComments lists the comments on the pull request whose head is branch;
// found is false when there is none.
func branchComments(ctx context.Context, vc versioncontrol.VersionControlAdapter, repo, branch string) (comments []versioncontrol.PullRequestComment, found bool, err error) {
	prs, err := vc.ListPRs(ctx, repo, time.Now().UTC().Add(-gitObservationWindow))
	if err != nil {
		return nil, false, err
	}
	for _, pr := range prs {
		if pr.HeadBranch == branch {
			c, err := vc.ListPullRequestComments(ctx, repo, pr.Number)
			return c, true, err
		}
	}
	return nil, false, nil
}
