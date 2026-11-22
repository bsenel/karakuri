package software

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 11: the git observation lists each open stream pull request
// whose head has no review, so the review pass is woken by it. A forge that
// cannot say which commits were reviewed leaves the list out and says why
// (AGENTS.md rule 10).

// reviewForge is a branchForge that answers ReviewedCommits from reviewed, a
// table of pull request id to the commit ids its reviews were left at.
type reviewForge struct {
	branchForge
	reviewed    map[string][]string
	reviewedErr error
}

func (f *reviewForge) ReviewedCommits(_ context.Context, _, prID string) ([]string, error) {
	if f.reviewedErr != nil {
		return nil, f.reviewedErr
	}
	return f.reviewed[prID], nil
}

func reviewPR(id, branch, head string) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{
		ID: id, Title: "title " + id, URL: "https://example.test/pull/" + id,
		HeadBranch: branch, HeadSHA: head, UpdatedAt: time.Now(),
	}
}

type prWithoutReview struct {
	PR      string `json:"pr"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Branch  string `json:"branch"`
	HeadSHA string `json:"head_sha"`
	Stream  string `json:"stream"`
}

func observeReviewForge(t *testing.T, f *reviewForge) environment.Observation {
	t.Helper()
	obs, err := (&gitEnv{id: EnvGit, vc: f}).Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	return obs
}

func prsWithoutReview(t *testing.T, state map[string]any) []prWithoutReview {
	t.Helper()
	raw, ok := state["prs_without_review"]
	if !ok {
		t.Fatalf("observation has no prs_without_review; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal prs_without_review: %v", err)
	}
	var out []prWithoutReview
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("prs_without_review is not a list of {pr, title, url, branch, head_sha, stream}: %v (%s)", err, b)
	}
	return out
}

func prReviewLines(t *testing.T, state map[string]any) []string {
	t.Helper()
	raw, ok := state["prs_without_review_lines"]
	if !ok {
		t.Fatalf("observation has no prs_without_review_lines; state keys: %v", stateKeys(state))
	}
	b, _ := json.Marshal(raw)
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("prs_without_review_lines is not a list of strings: %v (%s)", err, b)
	}
	return out
}

// A stream's open pull request with no review is listed with its stream and
// head, as a field and as a planner line.
func TestPRsWithoutReviewListsAStreamPullRequestWithNoReview(t *testing.T) {
	branch := "karakuri/phase-40-step-11-review-builds-branch"
	f := &reviewForge{}
	f.prs = []versioncontrol.PRSummary{reviewPR("273", branch, "head273")}

	obs := observeReviewForge(t, f)
	if msg, ok := obs.State["prs_without_review_error"]; ok {
		t.Fatalf("prs_without_review_error = %v on a forge that answered", msg)
	}
	got := prsWithoutReview(t, obs.State)
	want := prWithoutReview{
		PR: "273", Title: "title 273", URL: "https://example.test/pull/273",
		Branch: branch, HeadSHA: "head273", Stream: roadmapDeliveryTemplateID,
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("prs_without_review = %+v, want [%+v]", got, want)
	}
	lines := prReviewLines(t, obs.State)
	if len(lines) != 1 || !strings.Contains(lines[0], "273") {
		t.Errorf("prs_without_review_lines = %q, want one line naming pull request 273", lines)
	}
}

// A pull request whose branch no stream owns is not the reviewer's to review.
func TestPRsWithoutReviewSkipsABranchNoStreamOwns(t *testing.T) {
	f := &reviewForge{}
	f.prs = []versioncontrol.PRSummary{
		reviewPR("1", "someone/feature", "h1"),
		reviewPR("2", "main", "h2"),
	}
	got := prsWithoutReview(t, observeReviewForge(t, f).State)
	if len(got) != 0 {
		t.Errorf("prs_without_review = %+v, want none", got)
	}
}

// A head that was reviewed is not listed; one reviewed only at an older commit
// is, because the review does not cover what was pushed since.
func TestPRsWithoutReviewComparesTheHeadWithTheReviewedCommits(t *testing.T) {
	f := &reviewForge{reviewed: map[string][]string{
		"10": {"old", "head10"},
		"11": {"old11"},
	}}
	f.prs = []versioncontrol.PRSummary{
		reviewPR("10", "karakuri/phase-40-step-11-a", "head10"),
		reviewPR("11", "karakuri/phase-40-step-11-b", "head11"),
	}
	got := prsWithoutReview(t, observeReviewForge(t, f).State)
	if len(got) != 1 || got[0].PR != "11" {
		t.Errorf("prs_without_review = %+v, want only pull request 11 (reviewed at an older commit)", got)
	}
}

// Rule 10: a forge that cannot read the reviews is blind, not empty.
func TestPRsWithoutReviewSaysSoWhenReviewsCannotBeRead(t *testing.T) {
	for name, err := range map[string]error{
		"unsupported": versioncontrol.ErrUnsupported,
		"failed":      errors.New("boom"),
	} {
		t.Run(name, func(t *testing.T) {
			f := &reviewForge{reviewedErr: err}
			f.prs = []versioncontrol.PRSummary{reviewPR("273", "karakuri/phase-40-step-11-x", "h")}
			obs := observeReviewForge(t, f)
			if _, ok := obs.State["prs_without_review"]; ok {
				t.Errorf("prs_without_review is set when the reviews could not be read: %v", obs.State["prs_without_review"])
			}
			if msg, _ := obs.State["prs_without_review_error"].(string); msg == "" {
				t.Errorf("prs_without_review_error is not set; state keys: %v", stateKeys(obs.State))
			}
			lines := prReviewLines(t, obs.State)
			joined := strings.ToLower(strings.Join(lines, " "))
			if len(lines) == 0 || !strings.Contains(joined, "could not be read") || !strings.Contains(joined, "empty") {
				t.Errorf("lines = %q, want one saying the list could not be read and must not be taken as empty", lines)
			}
		})
	}
}
