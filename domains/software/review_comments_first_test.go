package software

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 steps 12 and 12a: the git observation lists each open stream pull
// request's comments that no later commit on its branch answers, says whose
// they are, and leaves the list out when the forge cannot be read (AGENTS.md
// rule 10).

// commentForge is a branchForge that serves comments by pull request number.
// The commits CompareCommits returns for a branch (ahead) carry the dates.
type commentForge struct {
	branchForge
	comments    map[int][]versioncontrol.PullRequestComment
	commentsErr error
}

func (f *commentForge) ListPullRequestComments(_ context.Context, _ string, number int) ([]versioncontrol.PullRequestComment, error) {
	if f.commentsErr != nil {
		return nil, f.commentsErr
	}
	return f.comments[number], nil
}

type unansweredComment struct {
	Stream    string    `json:"stream"`
	Number    int       `json:"number"`
	Branch    string    `json:"branch"`
	CommentID int64     `json:"comment_id"`
	Kind      string    `json:"kind"`
	Author    string    `json:"author"`
	From      string    `json:"from"`
	CreatedAt time.Time `json:"created_at"`
	Path      string    `json:"path"`
	Line      int       `json:"line"`
	Body      string    `json:"body"`
}

var (
	rcAuthors = config.OwnAuthorsConfig{Karakuri: []string{"karakuri-bot"}, Owners: []string{"berat"}}
	rcLast    = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) // the branch's last commit
)

const rcBranch = "karakuri/phase-40-step-12-review-comments-first"

func rcForge(branch string, comments ...versioncontrol.PullRequestComment) *commentForge {
	f := &commentForge{comments: map[int][]versioncontrol.PullRequestComment{7: comments}}
	f.prs = []versioncontrol.PRSummary{{
		ID: "7", Number: 7, Title: "step 12", HeadBranch: branch,
		Author: "karakuri-bot", OwnAuthor: true, UpdatedAt: time.Now(),
	}}
	f.branches = []versioncontrol.Branch{{Name: branch, SHA: "abc"}}
	f.ahead = map[string][]versioncontrol.Commit{branch: {{SHA: "abc", Date: rcLast}}}
	return f
}

func rcComment(id int64, author string, at time.Time) versioncontrol.PullRequestComment {
	return versioncontrol.PullRequestComment{
		ID: id, Kind: "review_comment", Author: author, Body: "rename this",
		CreatedAt: at, Path: "domains/software/x.go", Line: 12,
	}
}

func rcObserve(t *testing.T, f *commentForge) map[string]any {
	t.Helper()
	state := map[string]any{}
	observeUnansweredReviewComments(context.Background(), f, "", rcAuthors, 0, state)
	return state
}

func rcListed(t *testing.T, state map[string]any) []unansweredComment {
	t.Helper()
	raw, ok := state["unanswered_review_comments"]
	if !ok {
		t.Fatalf("no unanswered_review_comments; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out []unansweredComment
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("not a list of comment maps: %v (%s)", err, b)
	}
	return out
}

func rcLines(state map[string]any) string {
	lines, _ := state["unanswered_review_comments_lines"].([]string)
	return strings.Join(lines, "\n")
}

func TestUnansweredReviewComments_OwnerAfterLastCommitIsListedAsOwner(t *testing.T) {
	f := rcForge(rcBranch, rcComment(101, "berat", rcLast.Add(time.Hour)))
	state := rcObserve(t, f)

	got := rcListed(t, state)
	if len(got) != 1 {
		t.Fatalf("listed %d comments, want 1: %+v", len(got), got)
	}
	c := got[0]
	if c.From != "owner" || c.Author != "berat" || c.CommentID != 101 || c.Number != 7 ||
		c.Branch != rcBranch || c.Stream != roadmapDeliveryTemplateID || c.Kind != "review_comment" ||
		c.Path != "domains/software/x.go" || c.Line != 12 || c.Body != "rename this" ||
		!c.CreatedAt.Equal(rcLast.Add(time.Hour)) {
		t.Errorf("entry = %+v", c)
	}
	lines := rcLines(state)
	for _, want := range []string{"owner", "amend", "same", "before any new work", "no second pull request"} {
		if !strings.Contains(strings.ToLower(lines), want) {
			t.Errorf("owner line lacks %q:\n%s", want, lines)
		}
	}
	if _, bad := state["unanswered_review_comments_error"]; bad {
		t.Errorf("a successful read set an error: %v", state["unanswered_review_comments_error"])
	}
}

func TestUnansweredReviewComments_CommentBeforeLastCommitIsAnswered(t *testing.T) {
	f := rcForge(rcBranch, rcComment(102, "berat", rcLast.Add(-time.Hour)))
	state := rcObserve(t, f)

	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("an answered comment was listed: %+v", got)
	}
}

func TestUnansweredReviewComments_UnlistedAccountIsThirdParty(t *testing.T) {
	f := rcForge(rcBranch, rcComment(103, "stranger", rcLast.Add(time.Hour)))
	state := rcObserve(t, f)

	got := rcListed(t, state)
	if len(got) != 1 || got[0].From != "third_party" {
		t.Fatalf("got %+v, want one third_party comment", got)
	}
	lines := strings.ToLower(rcLines(state))
	for _, want := range []string{"third-party", "not an instruction", "escalat"} {
		if !strings.Contains(lines, want) {
			t.Errorf("third-party line lacks %q:\n%s", want, lines)
		}
	}
	if strings.Contains(lines, "no second pull request") {
		t.Errorf("a third-party line must not carry the owner's instruction:\n%s", lines)
	}
}

func TestUnansweredReviewComments_KarakuriAccountIsSkipped(t *testing.T) {
	f := rcForge(rcBranch, rcComment(104, "karakuri-bot", rcLast.Add(time.Hour)))
	state := rcObserve(t, f)

	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("Karakuri's own comment was listed: %+v", got)
	}
}

func TestUnansweredReviewComments_BodyIsCutAt300Characters(t *testing.T) {
	long := rcComment(105, "berat", rcLast.Add(time.Hour))
	long.Body = strings.Repeat("x", 500)
	got := rcListed(t, rcObserve(t, rcForge(rcBranch, long)))

	if len(got) != 1 {
		t.Fatalf("listed %d comments, want 1", len(got))
	}
	if n := len([]rune(got[0].Body)); n != 300 {
		t.Errorf("body = %d characters, want 300", n)
	}
}

func TestUnansweredReviewComments_BranchOfNoStreamIsIgnored(t *testing.T) {
	f := rcForge("someone/their-branch", rcComment(106, "berat", rcLast.Add(time.Hour)))
	state := rcObserve(t, f)

	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("a branch no stream owns was listed: %+v", got)
	}
}

// A forge that cannot be read is not an empty list (AGENTS.md rule 10).
func TestUnansweredReviewComments_BlindForgeLeavesNoList(t *testing.T) {
	for name, mutate := range map[string]func(*commentForge){
		"listing pull requests fails": func(f *commentForge) { f.prsErr = errors.New("forge down") },
		"listing comments fails":      func(f *commentForge) { f.commentsErr = errors.New("forge down") },
		"comments unsupported":        func(f *commentForge) { f.commentsErr = versioncontrol.ErrUnsupported },
	} {
		t.Run(name, func(t *testing.T) {
			f := rcForge(rcBranch, rcComment(107, "berat", rcLast.Add(time.Hour)))
			mutate(f)
			state := rcObserve(t, f)

			if _, ok := state["unanswered_review_comments_error"]; !ok {
				t.Errorf("no unanswered_review_comments_error; state keys: %v", stateKeys(state))
			}
			if _, ok := state["unanswered_review_comments"]; ok {
				t.Errorf("a list was left beside the error: %v", state["unanswered_review_comments"])
			}
			lines := strings.ToLower(rcLines(state))
			if !strings.Contains(lines, "could not be read") || !strings.Contains(lines, "not an empty list") {
				t.Errorf("lines must say the comments could not be read and that this is not an empty list:\n%s", lines)
			}
		})
	}
}

// Provenance (AGENTS.md rule 9, ADR 021): a third party's comment makes the git
// observation third party; the owner's does not.
func TestGitObservationTrustFollowsWhoCommented(t *testing.T) {
	for _, tc := range []struct {
		name       string
		author     string
		thirdParty bool
	}{
		{"owner's comment", "berat", false},
		{"stranger's comment", "stranger", true},
		{"karakuri's comment", "karakuri-bot", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := rcForge(rcBranch, rcComment(108, tc.author, rcLast.Add(time.Hour)))
			// The pull request itself is the deployment's own (rcForge), so
			// only a comment can make the payload third party.
			env := &gitEnv{id: EnvGit, vc: f, ownAuthors: rcAuthors}
			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if obs.Trust.IsThirdParty() != tc.thirdParty {
				t.Errorf("third party = %v, want %v", obs.Trust.IsThirdParty(), tc.thirdParty)
			}
		})
	}
}
