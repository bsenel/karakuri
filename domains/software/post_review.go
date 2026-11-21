package software

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// CapPostReview posts one comment review on the open pull request on
// params.branch (Phase 40 step 11).
const CapPostReview capability.CapabilityID = "software.act.post_review"

var (
	reviewRan    = regexp.MustCompile(`(?i)what was run`)
	reviewNotRan = regexp.MustCompile(`(?i)what was not run`)
)

// postReview posts params.body as one comment review. A review that does not
// say both what it ran and what it did not is refused before anything is read
// from the forge, so a reader never mistakes a partial check for a full one.
func (e *gitEnv) postReview(ctx context.Context, a environment.Action) environment.ActionResult {
	fail := func(msg string, trust environment.Trust, delta map[string]any) environment.ActionResult {
		return environment.ActionResult{Success: false, Trust: trust, StateDelta: delta,
			Error: fmt.Sprintf("%s: %s", CapPostReview, msg)}
	}
	if e.vc == nil || !e.vc.Active() {
		return fail("no version control instance is bound", environment.TrustOperator, nil)
	}
	branch := asString(a.Params, "branch")
	if branch == "" {
		return fail("needs the pull request's branch: pass params.branch", environment.TrustOperator, nil)
	}
	body := asString(a.Params, "body")
	if !reviewRan.MatchString(body) {
		return fail(`the review has no "What was run" section`, environment.TrustOperator, nil)
	}
	if !reviewNotRan.MatchString(body) {
		return fail(`the review has no "What was not run" section`, environment.TrustOperator, nil)
	}
	repo := asString(a.Params, "repo")

	pr, found, err := e.branchPR(ctx, repo, branch, time.Time{})
	if err != nil {
		return fail("could not list pull requests: "+err.Error(), environment.TrustOperator, nil)
	}
	if !found {
		return fail("no open pull request on "+branch, environment.TrustOperator, nil)
	}
	trust := prTrust([]versioncontrol.PRSummary{pr})
	delta := prRef(pr)
	if err := e.vc.PostReview(ctx, repo, pr.ID, body, nil); err != nil {
		if errors.Is(err, versioncontrol.ErrUnsupported) {
			return fail("unsupported: the forge cannot post a review: "+err.Error(), trust, delta)
		}
		return fail("could not post the review on pull request "+pr.ID+": "+err.Error(), trust, delta)
	}
	delta["review_posted"] = true
	return environment.ActionResult{Success: true, Trust: trust, StateDelta: delta}
}
