package report

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// failingReads wraps a real store and fails the reads the oversight view
// takes its refusals and blocks from, as a database in trouble would.
type failingReads struct {
	storage.StorageAdapter
	toolEvents error
	outcomes   error
}

func (f failingReads) ListToolEvents(ctx context.Context, filter storage.ToolEventFilter) ([]storage.ToolEvent, error) {
	if f.toolEvents != nil {
		return nil, f.toolEvents
	}
	return f.StorageAdapter.ListToolEvents(ctx, filter)
}

func (f failingReads) ListReconcileOutcomes(ctx context.Context, id objective.ObjectiveID, limit int) ([]reconcile.Outcome, error) {
	if f.outcomes != nil {
		return nil, f.outcomes
	}
	return f.StorageAdapter.ListReconcileOutcomes(ctx, id, limit)
}

// A read that failed is not a stream with no refusals and no blocks: the view
// says which read failed, as the waiting pull requests already do (AGENTS.md
// rule 10).
func TestAnUnreadableStoreIsReportedAsABlockNotAsNoRefusals(t *testing.T) {
	cases := []struct {
		name   string
		wrap   func(storage.StorageAdapter) storage.StorageAdapter
		read   string
		errMsg string
	}{
		{
			name: "tool events",
			wrap: func(s storage.StorageAdapter) storage.StorageAdapter {
				return failingReads{StorageAdapter: s, toolEvents: errors.New("audit table locked")}
			},
			read:   "audit log could not be read: ",
			errMsg: "audit table locked",
		},
		{
			name: "reconcile outcomes",
			wrap: func(s storage.StorageAdapter) storage.StorageAdapter {
				return failingReads{StorageAdapter: s, outcomes: errors.New("outcomes table locked")}
			},
			read:   "reconcile outcomes could not be read: ",
			errMsg: "outcomes table locked",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, clock := newService(t)
			seedTwin(t, store, "twin-1", "CTO twin")
			seedStanding(t, store, "twin-1", "obj-1", "keep the build green")
			svc.store = tc.wrap(store)

			d, err := svc.Assemble(context.Background(), "twin-1", clock.Add(-24*time.Hour), *clock)
			if err != nil {
				t.Fatalf("assemble: %v", err)
			}
			s := summaryOf(t, d, "obj-1")
			var found *digest.Block
			for i, b := range s.Blocks {
				if b.Kind == digest.BlockRemoteUnreadable && strings.HasPrefix(b.Reason, tc.read) {
					found = &s.Blocks[i]
				}
			}
			if found == nil {
				t.Fatalf("blocks = %+v, refusals = %+v: want a remote_unreadable block starting %q; an empty list must not read as no refusals and no blocks", s.Blocks, s.Refusals, tc.read)
			}
			if !strings.Contains(found.Reason, tc.errMsg) {
				t.Errorf("block reason = %q, want the error text %q", found.Reason, tc.errMsg)
			}
		})
	}
}
