package versioncontrol

import (
	"context"
	"time"
)

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
	Active() bool
}
