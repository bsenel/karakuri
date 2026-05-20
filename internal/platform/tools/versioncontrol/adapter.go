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
