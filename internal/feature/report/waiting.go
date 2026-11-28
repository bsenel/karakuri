package report

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// PullRequestReader reads the pull requests of a twin's streams from the
// forge. The digest takes it by injection so this package imports no vendor
// and no domain pack.
type PullRequestReader interface {
	// StreamPullRequests returns every pull request the forge holds for the
	// twin's streams, in any state, each tagged with the stream's objective.
	// A forge that cannot be read returns an error, never an empty list
	// (AGENTS.md rule 10).
	StreamPullRequests(ctx context.Context, twinID string) ([]StreamPullRequest, error)
}

// StreamPullRequest is one pull request as the reader found it.
type StreamPullRequest struct {
	ObjectiveID string
	Number      int
	Title       string
	URL         string
	// State is "open", "merged" or "closed".
	State string
	Draft bool
	// Review is the latest review's state: "approved", "changes_requested",
	// "commented", or "none".
	Review string
	// ReviewedHead reports whether that review was left at the current head.
	ReviewedHead bool
}

// readPullRequests reads the twin's stream pull requests once for the whole
// digest. No reader is an error, not an empty list: a digest that says nothing
// waits when it cannot see would be a lie (AGENTS.md rule 10).
func (s *Service) readPullRequests(ctx context.Context, twinID string) ([]StreamPullRequest, error) {
	if s.pulls == nil {
		return nil, errors.New("no forge is wired to read the pull requests")
	}
	return s.pulls.StreamPullRequests(ctx, twinID)
}

// waitingFor lists the open, non-draft pull requests of one stream by number.
// When they could not be read it lists none and returns the block that says so.
func waitingFor(prs []StreamPullRequest, readErr error, objectiveID objective.ObjectiveID, at time.Time) ([]digest.WaitingPullRequest, *digest.Block) {
	if readErr != nil {
		return nil, &digest.Block{
			Kind:   digest.BlockRemoteUnreadable,
			Reason: "pull requests could not be read: " + readErr.Error(),
			At:     at,
		}
	}
	var out []digest.WaitingPullRequest
	for _, p := range prs {
		if p.ObjectiveID != string(objectiveID) || p.State != "open" || p.Draft {
			continue
		}
		out = append(out, digest.WaitingPullRequest{
			Number: p.Number, Title: p.Title, URL: p.URL,
			Review: p.Review, ReviewedHead: p.ReviewedHead,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// Option configures a Service.
type Option func(*Service)

// WithPullRequestReader lets the digest list the pull requests waiting for a
// merge.
func WithPullRequestReader(r PullRequestReader) Option {
	return func(s *Service) { s.pulls = r }
}
