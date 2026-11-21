package software

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/platform/storage"
)

// newPostedCommentReader reads, from the audit log, the IDs of the comments
// Karakuri is recorded as having posted (Phase 40 step 12a). Scaffold: the
// tests in posted_comments_test.go describe its behaviour.
func newPostedCommentReader(_ storage.StorageAdapter) postedCommentIDs {
	return func(context.Context) (map[int64]bool, error) {
		return nil, errors.New("not implemented")
	}
}
