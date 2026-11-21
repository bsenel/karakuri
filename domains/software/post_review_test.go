package software

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 11: the reviewer posts one comment review on the open pull
// request of a stream's branch, and each review states what it ran and what it
// did not.
const capPostReview capability.CapabilityID = "software.act.post_review"

// reviewBody is a review that states both: what it ran, with the command and
// its output, and what it could not.
const reviewBody = `Finding: the branch does not build.

## What was run

- ` + "`go build ./...`" + ` exited 2: undefined: Frobnicate

## What was not run

- The web bundle: no node toolchain in this worktree.`

func actPostReview(t *testing.T, forge *fakeForge, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := (&gitEnv{id: EnvGit, vc: forge}).Act(context.Background(), environment.Action{CapabilityID: capPostReview, Params: params})
	if err != nil {
		t.Fatalf("act %s: %v", capPostReview, err)
	}
	return res
}

// One review is posted, on the pull request the branch heads, with the body
// the params carry.
func TestPostReviewPostsOneReviewOnTheBranchsPullRequest(t *testing.T) {
	forge := &fakeForge{prs: []versioncontrol.PRSummary{draftPR("body")}}
	res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "repo": "acme/widgets", "body": reviewBody})

	if !res.Success {
		t.Fatalf("Success = false (error %q), want a review posted", res.Error)
	}
	if len(forge.reviews) != 1 {
		t.Fatalf("PostReview calls = %d, want 1", len(forge.reviews))
	}
	got := forge.reviews[0]
	if got.repo != "acme/widgets" || got.prID != "201" || got.body != reviewBody {
		t.Errorf("review = {repo %q, pr %q, body %q}, want acme/widgets, 201 and the given body", got.repo, got.prID, got.body)
	}
	if fmt.Sprint(res.StateDelta["pr"]) != "201" {
		t.Errorf("state_delta pr = %v, want 201", res.StateDelta["pr"])
	}
}

// Both sections are required, in any case, and a review that leaves either out
// fails without posting anything.
func TestPostReviewRequiresWhatWasRunAndWhatWasNotRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{name: "both sections", body: reviewBody, ok: true},
		{name: "lower case headings", body: "what was run: go build\n\nwhat was not run: web bundle", ok: true},
		{name: "only what was run", body: "## What was run\n\n- go build ./... exited 0"},
		{name: "only what was not run", body: "## What was not run\n\n- the web bundle"},
		{name: "neither", body: "Looks fine to me."},
		{name: "empty", body: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := &fakeForge{prs: []versioncontrol.PRSummary{draftPR("body")}}
			res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "body": tc.body})

			if res.Success != tc.ok {
				t.Fatalf("Success = %v (error %q), want %v", res.Success, res.Error, tc.ok)
			}
			if !tc.ok {
				if len(forge.reviews) != 0 {
					t.Errorf("posted %d review(s) for a body that fails", len(forge.reviews))
				}
				if !strings.Contains(strings.ToLower(res.Error), "what was") {
					t.Errorf("error %q does not name the missing section", res.Error)
				}
			}
		})
	}
}

// The branch is required, and without it nothing is listed or posted.
func TestPostReviewRequiresABranch(t *testing.T) {
	forge := &fakeForge{prs: []versioncontrol.PRSummary{draftPR("body")}}
	res := actPostReview(t, forge, map[string]any{"body": reviewBody})

	if res.Success {
		t.Error("Success = true without params.branch")
	}
	if !strings.Contains(res.Error, "params.branch") {
		t.Errorf("error %q does not ask for params.branch", res.Error)
	}
	if len(forge.reviews) != 0 {
		t.Errorf("posted %d review(s) without a branch", len(forge.reviews))
	}
}

// A branch with no open pull request is a failure, and so is one whose pull
// request is on another branch.
func TestPostReviewFailsWithoutAnOpenPullRequestOnTheBranch(t *testing.T) {
	other := draftPR("body")
	other.HeadBranch = "karakuri/another-branch"
	for name, prs := range map[string][]versioncontrol.PRSummary{
		"no pull requests": nil,
		"another branch's": {other},
	} {
		t.Run(name, func(t *testing.T) {
			forge := &fakeForge{prs: prs}
			res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "body": reviewBody})

			if res.Success {
				t.Fatal("Success = true with no open pull request on the branch")
			}
			if !strings.Contains(res.Error, "no open pull request") {
				t.Errorf("error %q does not say there is no open pull request", res.Error)
			}
			if len(forge.reviews) != 0 {
				t.Errorf("posted %d review(s) with no pull request to post on", len(forge.reviews))
			}
		})
	}
}

// A forge that cannot post a review says so (AGENTS.md rule 10), and that is
// never a success.
func TestPostReviewReportsAnUnsupportedForge(t *testing.T) {
	forge := &fakeForge{
		prs:           []versioncontrol.PRSummary{draftPR("body")},
		postReviewErr: fmt.Errorf("fake_forge: post review: %w", versioncontrol.ErrUnsupported),
	}
	res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "body": reviewBody})

	if res.Success {
		t.Error("Success = true when the forge cannot post a review")
	}
	if !strings.Contains(strings.ToLower(res.Error), "unsupported") {
		t.Errorf("error %q does not say unsupported", res.Error)
	}
}

// Any other refusal by the forge is a failure too: the review was not posted.
func TestPostReviewReportsAForgeThatRefused(t *testing.T) {
	forge := &fakeForge{
		prs:           []versioncontrol.PRSummary{draftPR("body")},
		postReviewErr: fmt.Errorf("fake_forge: 422"),
	}
	res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "body": reviewBody})

	if res.Success {
		t.Error("Success = true when the forge refused the review")
	}
	if !strings.Contains(res.Error, "422") {
		t.Errorf("error %q does not carry the forge's reason", res.Error)
	}
}

// With no forge bound there is nothing to post on, which is an error and not
// an empty success.
func TestPostReviewWithNoForgeBoundFails(t *testing.T) {
	res, err := (&gitEnv{id: EnvGit, vc: versioncontrol.NewNoOp()}).Act(context.Background(),
		environment.Action{CapabilityID: capPostReview, Params: map[string]any{"branch": streamBranch, "body": reviewBody}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Error("Success = true with no version control instance bound")
	}
}

// post_review is served where mark_ready is, by the git environment that holds
// the version control adapter, and the pack declares it with the branch and the
// body as required inputs.
func TestPostReviewRoutesToGitAndIsDeclared(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	if got := reg.ServedBy(capPostReview); !reflect.DeepEqual(got, []environment.EnvironmentID{EnvGit}) {
		t.Errorf("%s is served by %v, want only %s", capPostReview, got, EnvGit)
	}

	var declared *capability.Capability
	for _, c := range New().Capabilities() {
		if c.ID == capPostReview {
			c := c
			declared = &c
		}
	}
	if declared == nil {
		t.Fatalf("%s is not declared", capPostReview)
	}
	for _, name := range []string{"branch", "body"} {
		found := false
		for _, r := range declared.InputSchema.Required {
			found = found || r == name
		}
		if !found {
			t.Errorf("Required = %v, want %s", declared.InputSchema.Required, name)
		}
	}
	if _, ok := declared.InputSchema.Properties["repo"]; !ok {
		t.Errorf("repo is not an optional input (properties %v)", declared.InputSchema.Properties)
	}
}
