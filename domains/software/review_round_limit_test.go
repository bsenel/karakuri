package software

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 12: after a declared number of rounds (a third-party comment
// answered by a later commit) the stream stops pushing and waits for the owner.
// Step 12a: the owner's comments do not count. Helpers come from
// review_comments_first_test.go.

func rcObserveLimit(t *testing.T, f *commentForge, limit int) map[string]any {
	t.Helper()
	state := map[string]any{}
	observeUnansweredReviewComments(context.Background(), f, "", rcAuthors, limit, state)
	return state
}

func rcWaiting(state map[string]any) []string {
	lines, _ := state["waiting_for_owner_lines"].([]string)
	return lines
}

// rcRounds builds a pull request with n comments by author, each answered by a
// later commit, and one new comment by newAuthor after the last commit.
func rcRounds(n int, author, newAuthor string) *commentForge {
	var comments []versioncontrol.PullRequestComment
	var commits []versioncontrol.Commit
	for i := 0; i < n; i++ {
		at := rcLast.Add(time.Duration(i-n-1) * 2 * time.Hour)
		comments = append(comments, rcComment(int64(200+i), author, at))
		commits = append(commits, versioncontrol.Commit{SHA: "c" + string(rune('a'+i)), Date: at.Add(time.Hour)})
	}
	comments = append(comments, rcComment(300, newAuthor, rcLast.Add(time.Hour)))
	f := rcForge(rcBranch, comments...)
	f.ahead = map[string][]versioncontrol.Commit{rcBranch: append(commits, versioncontrol.Commit{SHA: "last", Date: rcLast})}
	return f
}

func TestReviewRoundLimit_PastLimitListsNothingAndWaitsOnce(t *testing.T) {
	state := rcObserveLimit(t, rcRounds(3, "stranger", "stranger"), 3)

	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("past the limit the comment was still listed: %+v", got)
	}
	if strings.Contains(rcLines(state), "no later commit answers") {
		t.Errorf("past the limit the planner is still told to answer:\n%s", rcLines(state))
	}
	w := rcWaiting(state)
	if len(w) != 1 {
		t.Fatalf("waiting_for_owner_lines = %d entries, want 1: %v", len(w), w)
	}
	if !strings.Contains(w[0], "7") || !strings.Contains(w[0], rcBranch) || !strings.Contains(strings.ToLower(w[0]), "owner") {
		t.Errorf("waiting entry must name pull request 7 and %s: %q", rcBranch, w[0])
	}
}

func TestReviewRoundLimit_ValueChangesOutcome(t *testing.T) {
	f := rcRounds(1, "stranger", "stranger")
	if len(rcWaiting(rcObserveLimit(t, f, 1))) != 1 {
		t.Errorf("limit 1 with one answered round must wait for the owner")
	}
	state := rcObserveLimit(t, f, 2)
	if len(rcWaiting(state)) != 0 || len(rcListed(t, state)) != 1 {
		t.Errorf("limit 2 with one answered round must list the comment and not wait: %v %v", rcWaiting(state), rcListed(t, state))
	}
}

func TestReviewRoundLimit_OwnerCommentsDoNotCount(t *testing.T) {
	state := rcObserveLimit(t, rcRounds(3, "berat", "stranger"), 3)

	if got := rcListed(t, state); len(got) != 1 || got[0].CommentID != 300 {
		t.Errorf("owner rounds counted toward the limit; listed: %+v", got)
	}
	if w := rcWaiting(state); len(w) != 0 {
		t.Errorf("waiting entry after owner-only rounds: %v", w)
	}
}

func TestReviewRoundLimit_NewOwnerCommentPastLimitIsStillListed(t *testing.T) {
	state := rcObserveLimit(t, rcRounds(3, "stranger", "berat"), 3)

	if got := rcListed(t, state); len(got) != 1 || got[0].From != "owner" {
		t.Errorf("the owner's new comment must still be listed: %+v", got)
	}
}

func TestReviewRoundLimit_BelowLimitUnchanged(t *testing.T) {
	state := rcObserveLimit(t, rcRounds(2, "stranger", "stranger"), 3)

	if got := rcListed(t, state); len(got) != 1 || got[0].CommentID != 300 {
		t.Errorf("below the limit the comment must be listed: %+v", got)
	}
	if w := rcWaiting(state); len(w) != 0 {
		t.Errorf("waiting entry below the limit: %v", w)
	}
}

func TestReviewRoundLimit_GitEnvCarriesTheLimit(t *testing.T) {
	env := &gitEnv{id: EnvGit, vc: rcRounds(1, "stranger", "stranger"), ownAuthors: rcAuthors, reviewRoundLimit: 1}
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(obs)
	if !strings.Contains(string(b), "waiting_for_owner_lines") {
		t.Errorf("observation lacks waiting_for_owner_lines: %s", b)
	}
}
