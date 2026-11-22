package software

import (
	"context"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// The test doubles of the version control adapter that do not post reviews.

func (s *vcStub) PostReview(context.Context, string, string, string, []versioncontrol.ReviewComment) error {
	return versioncontrol.ErrUnsupported
}

func (f *branchForge) PostReview(context.Context, string, string, string, []versioncontrol.ReviewComment) error {
	return versioncontrol.ErrUnsupported
}

func (w *windowVC) PostReview(context.Context, string, string, string, []versioncontrol.ReviewComment) error {
	return versioncontrol.ErrUnsupported
}

// postedReview is one PostReview call.
type postedReview struct {
	repo, prID, body string
	comments         []versioncontrol.ReviewComment
}

func (f *fakeForge) PostReview(_ context.Context, repo, prID, body string, comments []versioncontrol.ReviewComment) error {
	f.reviews = append(f.reviews, postedReview{repo, prID, body, comments})
	return f.postReviewErr
}

func (s *vcStub) ListReviews(context.Context, string, string) ([]versioncontrol.Review, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (w *windowVC) ListReviews(context.Context, string, string) ([]versioncontrol.Review, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (f *fakeForge) ListReviews(context.Context, string, string) ([]versioncontrol.Review, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (s *vcStub) ReviewedCommits(context.Context, string, string) ([]string, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (w *windowVC) ReviewedCommits(context.Context, string, string) ([]string, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (f *fakeForge) ReviewedCommits(context.Context, string, string) ([]string, error) {
	return nil, versioncontrol.ErrUnsupported
}
