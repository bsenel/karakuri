package software

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/platform/storage"
)

// newLastCycleEnd is the discovery trigger's LastCycleEnd (Phase 40 step 13):
// the end time of the most recent storage.ToolEventRunEnd row of any objective
// made from templateID, whichever trigger started it; zero when there is none,
// an error when the log cannot be read (zero would read as no cycle ever).
func newLastCycleEnd(s storage.StorageAdapter, templateID string) func(context.Context) (time.Time, error) {
	return func(ctx context.Context) (time.Time, error) {
		events, err := s.ListToolEvents(ctx, storage.ToolEventFilter{
			Kind: storage.ToolEventRunEnd, TemplateID: templateID, Limit: 1,
		})
		if err != nil {
			return time.Time{}, fmt.Errorf("list run_end events: %w", err)
		}
		if len(events) == 0 {
			return time.Time{}, nil
		}
		return events[0].CreatedAt, nil
	}
}
