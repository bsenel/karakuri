package storage_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"gorm.io/gorm"
)

type windowRow struct {
	id string
	at time.Time
}

// seedWindowRows saves the rows in the order given and backdates each one,
// because SaveToolEvent stamps created_at on insert.
func seedWindowRows(t *testing.T, s *storage.GORMStorage, db *gorm.DB, rows []windowRow) {
	t.Helper()
	ctx := context.Background()
	for _, r := range rows {
		if err := s.SaveToolEvent(ctx, storage.ToolEvent{ID: r.id, ObjectiveID: "obj-1", Kind: storage.ToolEventExecute, Success: true}); err != nil {
			t.Fatalf("save %s: %v", r.id, err)
		}
		if err := db.Model(&schema.ToolEventModel{}).Where("id = ?", r.id).UpdateColumn("created_at", r.at).Error; err != nil {
			t.Fatalf("backdate %s: %v", r.id, err)
		}
	}
}

func windowIDs(t *testing.T, s *storage.GORMStorage, f storage.ToolEventFilter) []string {
	t.Helper()
	events, err := s.ListToolEvents(context.Background(), f)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	return ids
}

// An export window is [from, to): a row at from belongs to it and a row at to
// belongs to the next one, so two adjacent windows never share a row.
func TestListToolEventsWindowBounds(t *testing.T) {
	s, db := newStoreWithDB(t)
	from := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	seedWindowRows(t, s, db, []windowRow{
		{"before-from", from.Add(-time.Second)},
		{"at-from", from},
		{"inside", from.Add(time.Hour)},
		{"last-instant", to.Add(-time.Second)},
		{"at-to", to},
		{"after-to", to.Add(time.Hour)},
	})

	got := windowIDs(t, s, storage.ToolEventFilter{CreatedAtSince: &from, CreatedAtBefore: &to, OldestFirst: true})
	want := []string{"at-from", "inside", "last-instant"}
	if !slices.Equal(got, want) {
		t.Errorf("window rows = %v, want %v (from inclusive, to exclusive)", got, want)
	}

	// The upper bound stands on its own too.
	got = windowIDs(t, s, storage.ToolEventFilter{CreatedAtBefore: &from, OldestFirst: true})
	if want := []string{"before-from"}; !slices.Equal(got, want) {
		t.Errorf("rows before from = %v, want %v", got, want)
	}
}

// OldestFirst orders by (created_at, id), so two rows written in the same
// instant come back in one order however they were inserted.
func TestListToolEventsOldestFirstOrdersByTimeThenID(t *testing.T) {
	s, db := newStoreWithDB(t)
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	seedWindowRows(t, s, db, []windowRow{
		{"row-d", base.Add(3 * time.Hour)},
		{"tie-b", base.Add(time.Hour)},
		{"row-a", base},
		{"tie-a", base.Add(time.Hour)},
		{"row-c", base.Add(2 * time.Hour)},
		{"tie-c", base.Add(time.Hour)},
	})

	got := windowIDs(t, s, storage.ToolEventFilter{OldestFirst: true})
	want := []string{"row-a", "tie-a", "tie-b", "tie-c", "row-c", "row-d"}
	if !slices.Equal(got, want) {
		t.Errorf("oldest first = %v, want %v", got, want)
	}
}

// An export must not silently stop at a page: Limit 0 is every row.
func TestListToolEventsWindowLimitZeroReturnsEveryRow(t *testing.T) {
	s, db := newStoreWithDB(t)
	from := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	const inside = 150
	rows := make([]windowRow, 0, inside+2)
	for i := 0; i < inside; i++ {
		rows = append(rows, windowRow{fmt.Sprintf("in-%03d", i), from.Add(time.Duration(i) * time.Minute)})
	}
	rows = append(rows, windowRow{"out-before", from.Add(-time.Minute)}, windowRow{"out-after", to})
	seedWindowRows(t, s, db, rows)

	got := windowIDs(t, s, storage.ToolEventFilter{CreatedAtSince: &from, CreatedAtBefore: &to, OldestFirst: true, Limit: 0})
	if len(got) != inside {
		t.Fatalf("rows in window = %d, want %d", len(got), inside)
	}
	if got[0] != "in-000" || got[inside-1] != fmt.Sprintf("in-%03d", inside-1) {
		t.Errorf("first, last = %s, %s; want in-000, in-%03d", got[0], got[inside-1], inside-1)
	}
}

// SQLite compares datetimes as text, so a bound carrying another zone's offset
// would be compared by its digits. The same instant must select the same rows
// whatever zone it is written in.
func TestListToolEventsWindowBoundsInAnotherZone(t *testing.T) {
	s, db := newStoreWithDB(t)
	from := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(6 * time.Hour)
	seedWindowRows(t, s, db, []windowRow{
		{"before-from", from.Add(-time.Hour)},
		{"at-from", from},
		{"one-hour-in", from.Add(time.Hour)},
		{"last-instant", to.Add(-time.Second)},
		{"at-to", to},
		{"after-to", to.Add(3 * time.Hour)},
	})
	want := []string{"at-from", "one-hour-in", "last-instant"}

	inUTC := windowIDs(t, s, storage.ToolEventFilter{CreatedAtSince: &from, CreatedAtBefore: &to, OldestFirst: true})
	if !slices.Equal(inUTC, want) {
		t.Errorf("UTC bounds = %v, want %v", inUTC, want)
	}
	for _, zone := range []*time.Location{time.FixedZone("east", 5*60*60), time.FixedZone("west", -8*60*60)} {
		zf, zt := from.In(zone), to.In(zone)
		got := windowIDs(t, s, storage.ToolEventFilter{CreatedAtSince: &zf, CreatedAtBefore: &zt, OldestFirst: true})
		if !slices.Equal(got, want) {
			t.Errorf("bounds in zone %s = %v, want %v (the same instants as UTC)", zone, got, want)
		}
	}
}

// The audit page reads newest first; the window fields must not change that.
func TestListToolEventsDefaultOrderIsNewestFirst(t *testing.T) {
	s, db := newStoreWithDB(t)
	from := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	seedWindowRows(t, s, db, []windowRow{
		{"second", from.Add(time.Hour)},
		{"first", from},
		{"third", from.Add(2 * time.Hour)},
		{"outside", to},
	})

	got := windowIDs(t, s, storage.ToolEventFilter{})
	if want := []string{"outside", "third", "second", "first"}; !slices.Equal(got, want) {
		t.Errorf("default order = %v, want %v (newest first)", got, want)
	}
	got = windowIDs(t, s, storage.ToolEventFilter{CreatedAtSince: &from, CreatedAtBefore: &to})
	if want := []string{"third", "second", "first"}; !slices.Equal(got, want) {
		t.Errorf("windowed default order = %v, want %v (newest first)", got, want)
	}
}
