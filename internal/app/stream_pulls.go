package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurereport "github.com/bsenel/karakuri/internal/feature/report"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// streamPullRequests reads a twin's stream pull requests over the version
// control instance the twin is bound to (Phase 40 step 14). It lives here
// because it joins the report feature, the software pack's branch rule and the
// tool registry, which none of them may import from one another.
type streamPullRequests struct {
	store storage.StorageAdapter
	tools *tools.Registry
}

var _ featurereport.PullRequestReader = streamPullRequests{}

// StreamPullRequests lists the open pull requests of the twin's streams. A
// pull request belongs to the standing objective whose template owns its head
// branch, the rule the git observation uses (domainsw.StreamOfBranch); one no
// stream owns is left out. An inactive forge or a failed read is an error, never
// an empty list (AGENTS.md rule 10).
func (r streamPullRequests) StreamPullRequests(ctx context.Context, twinID string) ([]featurereport.StreamPullRequest, error) {
	if r.tools == nil {
		return nil, errors.New("no tool registry")
	}
	t, err := r.store.GetTwin(ctx, twinID)
	if err != nil {
		return nil, fmt.Errorf("read twin: %w", err)
	}
	vc, ok := r.tools.VC.Resolve(t.AdapterBindings["versioncontrol"])
	if !ok || !vc.Active() {
		return nil, errors.New("no version control adapter is bound to the twin")
	}
	objectives, err := r.store.ListObjectives(ctx, storage.ObjectiveFilter{
		TwinID: twinID,
		Mode:   string(objective.ModeStanding),
	})
	if err != nil {
		return nil, fmt.Errorf("list objectives: %w", err)
	}
	byTemplate := map[string]objective.ObjectiveID{}
	for _, o := range objectives {
		if _, seen := byTemplate[o.TemplateID]; !seen && o.TemplateID != "" {
			byTemplate[o.TemplateID] = o.ID
		}
	}
	// A zero since lists every open pull request, not only the recent ones.
	prs, err := vc.ListPRs(ctx, "", time.Time{})
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	var out []featurereport.StreamPullRequest
	for _, pr := range prs {
		id, ok := byTemplate[domainsw.StreamOfBranch(pr.HeadBranch)]
		if !ok {
			continue
		}
		reviews, err := vc.ListReviews(ctx, "", pr.ID)
		if err != nil {
			return nil, fmt.Errorf("list reviews of #%s: %w", pr.ID, err)
		}
		state, atHead := latestReview(reviews, pr.HeadSHA)
		out = append(out, featurereport.StreamPullRequest{
			ObjectiveID: string(id), Number: pr.Number, Title: pr.Title, URL: pr.URL,
			State: "open", Draft: pr.Draft, Review: state, ReviewedHead: atHead,
		})
	}
	return out, nil
}

// latestReview is the state of the newest review that says something
// (approved, changes_requested, commented) and whether it was left at head;
// "none" when nobody has reviewed. Pending and dismissed reviews say nothing.
func latestReview(reviews []versioncontrol.Review, head string) (string, bool) {
	for i := len(reviews) - 1; i >= 0; i-- {
		switch reviews[i].State {
		case "approved", "changes_requested", "commented":
			return reviews[i].State, head != "" && reviews[i].CommitID == head
		}
	}
	return "none", false
}
