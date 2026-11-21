package software

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// newClosingComment is a stub until the production slice lands.
func newClosingComment(vc versioncontrol.VersionControlAdapter, ownAuthors config.OwnAuthorsConfig, posted postedCommentIDs) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		return "", errors.New("closing comment: not implemented")
	}
}
