package versioncontrol

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is what an adapter returns, wrapped with its own name, for a
// read its forge does not offer, never an empty result (AGENTS.md rule 10).
var ErrUnsupported = errors.New("read not supported by this version control adapter")

type PullRequest struct {
	Title        string
	Body         string
	HeadBranch   string
	BaseBranch   string
	WorktreePath string
}

type PRSummary struct {
	ID    string
	Title string
	URL   string

	// CheckState is the combined CI verdict on the pull request's head:
	// "success", "failure", "pending", or "" when the adapter could not tell.
	//
	// It travels on the summary rather than behind a second call because
	// "what is currently broken" is the question a list of open pull requests
	// is usually being asked, and an adapter that has the answer should not
	// make the caller fan out N requests to reassemble it.
	CheckState string
	// FailingChecks names the checks that are red, so a proposal can say what
	// is broken rather than that something is.
	FailingChecks []string

	// Author is the login that opened the pull request.
	Author string
	// OwnAuthor reports whether Author is one of the logins the operator
	// configured as this deployment's own (`own_authors` on the instance).
	// The title and body of such a pull request were written by the
	// deployment or its operator, not by a stranger. False when no list is
	// configured, so every pull request counts as somebody else's writing.
	OwnAuthor bool

	// HeadBranch and HeadSHA name the branch the pull request is opened from
	// and the commit at its tip.
	HeadBranch string
	HeadSHA    string
	// CreatedAt and UpdatedAt say whether the pull request was opened or
	// updated within a pass.
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CheckRun is one CI check as the forge reports it. HeadSHA is the commit it
// ran on, which may be older than the pull request's head.
type CheckRun struct {
	Name    string
	HeadSHA string
	// Status is "queued", "in_progress" or "completed".
	Status string
	// Conclusion is set once Status is "completed": "success", "failure",
	// "neutral", "skipped", "cancelled", "timed_out" or "action_required".
	Conclusion string
}

type Commit struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

type VersionControlAdapter interface {
	Name() string
	CreatePR(ctx context.Context, pr PullRequest) (prURL string, err error)
	ListPRs(ctx context.Context, repo string, since time.Time) ([]PRSummary, error)
	GetCommits(ctx context.Context, repo string, since time.Time) ([]Commit, error)
	// ListChecks returns the check runs the forge holds for ref, a branch or a
	// commit SHA.
	ListChecks(ctx context.Context, repo, ref string) ([]CheckRun, error)
	// ChangedFiles returns the paths a pull request's diff touches.
	ChangedFiles(ctx context.Context, repo, prID string) ([]string, error)
	Active() bool
}
