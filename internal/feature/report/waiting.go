package report

import "context"

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

// Option configures a Service.
type Option func(*Service)

// WithPullRequestReader lets the digest list the pull requests waiting for a
// merge.
func WithPullRequestReader(r PullRequestReader) Option {
	return func(s *Service) { s.pulls = r }
}
