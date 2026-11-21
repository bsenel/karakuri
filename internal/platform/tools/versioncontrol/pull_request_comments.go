package versioncontrol

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// commentsMaxPages bounds one endpoint's pagination, a hundred a page.
const commentsMaxPages = 30

// ListPullRequestComments reads everything written on a pull request: the
// conversation (issues/N/comments), inline review comments (pulls/N/comments)
// and review summaries (pulls/N/reviews), oldest first. A review with an empty
// body is skipped; it says nothing to answer. Any failing endpoint is an
// error, never a shorter list.
func (g *GitHub) ListPullRequestComments(ctx context.Context, repo string, number int) ([]PullRequestComment, error) {
	if repo == "" {
		repo = g.repo
	}
	if repo == "" {
		return nil, fmt.Errorf("github: no repo configured")
	}
	type rawComment struct {
		ID          int64     `json:"id"`
		Body        string    `json:"body"`
		CreatedAt   time.Time `json:"created_at"`
		SubmittedAt time.Time `json:"submitted_at"`
		Path        string    `json:"path"`
		Line        *int      `json:"line"` // null on an outdated inline comment
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	sources := []struct{ kind, resource, suffix string }{
		{"issue_comment", "issues", "comments"},
		{"review_comment", "pulls", "comments"},
		{"review", "pulls", "reviews"},
	}
	var out []PullRequestComment
	for _, src := range sources {
		base := fmt.Sprintf("/repos/%s/%s/%d/%s", repo, src.resource, number, src.suffix)
		done := false
		for page := 1; page <= commentsMaxPages; page++ {
			var raw []rawComment
			if err := g.do(ctx, "GET", fmt.Sprintf("%s?per_page=100&page=%d", base, page), nil, &raw); err != nil {
				return nil, fmt.Errorf("github: pull request comments: %w", err)
			}
			for _, r := range raw {
				at := r.CreatedAt
				if src.kind == "review" {
					if r.Body == "" {
						continue
					}
					at = r.SubmittedAt
				}
				c := PullRequestComment{ID: r.ID, Kind: src.kind, Author: r.User.Login, Body: r.Body, CreatedAt: at}
				if src.kind == "review_comment" {
					c.Path = r.Path
					if r.Line != nil {
						c.Line = *r.Line
					}
				}
				out = append(out, c)
			}
			if len(raw) < 100 {
				done = true
				break
			}
		}
		if !done {
			return nil, fmt.Errorf("github: pull request comments: %s has more pages than GitHub lists", base)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}
