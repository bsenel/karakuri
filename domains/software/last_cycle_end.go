package software

import (
	"context"
	"errors"
	"time"

	"github.com/bsenel/karakuri/internal/platform/storage"
)

// newLastCycleEnd is the discovery trigger's LastCycleEnd (Phase 40 step 13):
// the end time of the most recent storage.ToolEventRunEnd row of any objective
// made from templateID, whichever trigger started it; zero when there is none,
// an error when the log cannot be read.
//
// Scaffolding only: the behaviour is specified by last_cycle_end_test.go.
func newLastCycleEnd(_ storage.StorageAdapter, _ string) func(context.Context) (time.Time, error) {
	return func(context.Context) (time.Time, error) {
		return time.Time{}, errors.New("the end of the last discovery cycle is not read yet")
	}
}
