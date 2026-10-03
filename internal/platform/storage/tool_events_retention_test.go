package storage_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Pruning the audit log is the one place Karakuri destroys its own record, so
// the boundary has to be exact: a row at the cutoff is still inside the
// retention window and must survive.
func TestDeleteToolEventsBefore(t *testing.T) {
	ctx := context.Background()
	s, db := newStoreWithDB(t)

	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	events := []struct {
		id, kind string
		at       time.Time
	}{
		{"old-execute", storage.ToolEventExecute, cutoff.Add(-48 * time.Hour)},
		{"old-approval", storage.ToolEventApproval, cutoff.Add(-time.Hour)},
		{"old-rejection", storage.ToolEventRejection, cutoff.Add(-time.Second)},
		{"at-execute", storage.ToolEventExecute, cutoff},
		{"at-approval", storage.ToolEventApproval, cutoff},
		{"new-rejection", storage.ToolEventRejection, cutoff.Add(time.Second)},
		{"new-execute", storage.ToolEventExecute, cutoff.Add(48 * time.Hour)},
	}
	for _, e := range events {
		if err := s.SaveToolEvent(ctx, storage.ToolEvent{ID: e.id, ObjectiveID: "obj-1", Kind: e.kind, Success: true}); err != nil {
			t.Fatalf("save %s: %v", e.id, err)
		}
		// SaveToolEvent does not carry a timestamp; the column is stamped on
		// insert. Backdate it directly so the test owns the boundary.
		if err := db.Model(&schema.ToolEventModel{}).Where("id = ?", e.id).UpdateColumn("created_at", e.at).Error; err != nil {
			t.Fatalf("backdate %s: %v", e.id, err)
		}
	}

	n, err := s.DeleteToolEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted = %d, want 3 (rows strictly older than the cutoff)", n)
	}

	left, err := s.ListToolEvents(ctx, storage.ToolEventFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(left))
	for _, e := range left {
		got = append(got, e.ID)
	}
	slices.Sort(got)
	want := []string{"at-approval", "at-execute", "new-execute", "new-rejection"}
	if !slices.Equal(got, want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}

	again, err := s.DeleteToolEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if again != 0 {
		t.Errorf("second delete = %d, want 0", again)
	}
}

// What produced a decision is kept as columns, not inside the payload: an
// auditor asks "everything this model decided" across the whole log, and that
// is a WHERE clause or it is a scan of every row's JSON.
func TestToolEventProvenanceRoundTripsAsColumns(t *testing.T) {
	ctx := context.Background()
	s, db := newStoreWithDB(t)

	if err := s.SaveToolEvent(ctx, storage.ToolEvent{
		ID: "ev-1", ObjectiveID: "obj-1", AgentID: "agent-1", Kind: storage.ToolEventEscalation,
		Provider: "anthropic", Model: "model-a", TemplateID: "tmpl-green-build", AutonomyRung: "propose",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	listed, err := s.ListToolEvents(ctx, storage.ToolEventFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d rows, want 1", len(listed))
	}
	got := listed[0]
	if got.Provider != "anthropic" || got.Model != "model-a" {
		t.Errorf("provider/model = %q/%q, want anthropic/model-a", got.Provider, got.Model)
	}
	if got.TemplateID != "tmpl-green-build" {
		t.Errorf("template id = %q, want tmpl-green-build", got.TemplateID)
	}
	if got.AutonomyRung != "propose" {
		t.Errorf("autonomy rung = %q, want propose", got.AutonomyRung)
	}

	var cols struct{ Provider, Model, TemplateID, AutonomyRung string }
	if err := db.Raw("SELECT provider, model, template_id, autonomy_rung FROM tool_events WHERE id = ?", "ev-1").
		Scan(&cols).Error; err != nil {
		t.Fatalf("read the columns: %v", err)
	}
	if cols.Provider != "anthropic" || cols.Model != "model-a" ||
		cols.TemplateID != "tmpl-green-build" || cols.AutonomyRung != "propose" {
		t.Errorf("columns = %+v, want anthropic/model-a/tmpl-green-build/propose", cols)
	}
}

// Each filter is seeded with rows that differ in that one field and nothing
// else, so a filter that is accepted and ignored returns all three and fails.
func TestListToolEventsNarrowsByProvenance(t *testing.T) {
	base := storage.ToolEvent{
		ObjectiveID: "obj-1", AgentID: "agent-1", Kind: storage.ToolEventExecute, Success: true,
		Provider: "anthropic", Model: "model-a", TemplateID: "tmpl-green-build", AutonomyRung: "act",
	}
	cases := map[string]struct {
		set    func(e *storage.ToolEvent, v string)
		filter func(v string) storage.ToolEventFilter
		values [3]string // the first is the one asked for; the last is "unset"
	}{
		"provider": {
			set:    func(e *storage.ToolEvent, v string) { e.Provider = v },
			filter: func(v string) storage.ToolEventFilter { return storage.ToolEventFilter{Provider: v} },
			values: [3]string{"anthropic", "fallback", ""},
		},
		"model": {
			set:    func(e *storage.ToolEvent, v string) { e.Model = v },
			filter: func(v string) storage.ToolEventFilter { return storage.ToolEventFilter{Model: v} },
			values: [3]string{"model-a", "model-b", ""},
		},
		"template": {
			set:    func(e *storage.ToolEvent, v string) { e.TemplateID = v },
			filter: func(v string) storage.ToolEventFilter { return storage.ToolEventFilter{TemplateID: v} },
			values: [3]string{"tmpl-green-build", "tmpl-triage", ""},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)
			for i, v := range tc.values {
				e := base
				e.ID = []string{"wanted", "other", "unset"}[i]
				tc.set(&e, v)
				if err := s.SaveToolEvent(ctx, e); err != nil {
					t.Fatalf("save %s: %v", e.ID, err)
				}
			}

			got, err := s.ListToolEvents(ctx, tc.filter(tc.values[0]))
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			ids := make([]string, 0, len(got))
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			slices.Sort(ids)
			if !slices.Equal(ids, []string{"wanted"}) {
				t.Errorf("filtering %s by %q listed %v, want only [wanted]", name, tc.values[0], ids)
			}

			// An empty filter field means unfiltered, as every other field does.
			all, err := s.ListToolEvents(ctx, storage.ToolEventFilter{})
			if err != nil {
				t.Fatalf("list all: %v", err)
			}
			if len(all) != 3 {
				t.Errorf("unfiltered listing = %d rows, want 3", len(all))
			}
		})
	}
}

// Every row written before these columns existed names none of them. It must
// still list, and read back as "not recorded" rather than fail the scan.
func TestToolEventWrittenBeforeProvenanceReadsBackEmpty(t *testing.T) {
	ctx := context.Background()
	s, db := newStoreWithDB(t)

	if err := db.Exec(
		`INSERT INTO tool_events (id, objective_id, agent_id, capability, adapter, success, confidence, kind, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ev-old", "obj-1", "agent-1", "test.run", "software.env.ci", true, 0.9, storage.ToolEventExecute,
		time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	).Error; err != nil {
		t.Fatalf("insert the old row: %v", err)
	}

	listed, err := s.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: "obj-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "ev-old" {
		t.Fatalf("listed %+v, want the one old row", listed)
	}
	got := listed[0]
	if got.Provider != "" || got.Model != "" || got.TemplateID != "" || got.AutonomyRung != "" {
		t.Errorf("provider/model/template/rung = %q/%q/%q/%q, want all empty",
			got.Provider, got.Model, got.TemplateID, got.AutonomyRung)
	}

	// And the columns are there to be empty: without them this row would be
	// indistinguishable from one the schema simply cannot describe.
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM tool_events WHERE id = ? AND provider = '' AND model = '' AND template_id = '' AND autonomy_rung = ''", "ev-old").
		Scan(&n).Error; err != nil {
		t.Fatalf("read the columns: %v", err)
	}
	if n != 1 {
		t.Errorf("old row with empty provenance columns = %d, want 1", n)
	}
}

// The cutoff names an instant, not a wall-clock reading. SQLite compares
// datetimes as text, so a cutoff carried in another zone would be compared by
// its digits and delete rows newer than the instant it names: hours taken out
// of the retention floor. The delete must not depend on its caller passing UTC.
func TestDeleteToolEventsBeforeTakesTheCutoffAsAnInstant(t *testing.T) {
	ctx := context.Background()
	s, db := newStoreWithDB(t)

	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	for id, at := range map[string]time.Time{
		"older":         cutoff.Add(-time.Hour),
		"newer-by-1h":   cutoff.Add(time.Hour),
		"newer-by-4h":   cutoff.Add(4 * time.Hour),
		"newer-by-days": cutoff.Add(72 * time.Hour),
	} {
		if err := s.SaveToolEvent(ctx, storage.ToolEvent{ID: id, ObjectiveID: "obj-1", Kind: storage.ToolEventExecute, Success: true}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
		if err := db.Model(&schema.ToolEventModel{}).Where("id = ?", id).UpdateColumn("created_at", at).Error; err != nil {
			t.Fatalf("backdate %s: %v", id, err)
		}
	}

	// The same instant, read off a clock five hours ahead of UTC.
	elsewhere := cutoff.In(time.FixedZone("UTC+5", 5*60*60))
	n, err := s.DeleteToolEventsBefore(ctx, elsewhere)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1: only the row older than the instant", n)
	}
	left, err := s.ListToolEvents(ctx, storage.ToolEventFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(left))
	for _, e := range left {
		got = append(got, e.ID)
	}
	slices.Sort(got)
	if want := []string{"newer-by-1h", "newer-by-4h", "newer-by-days"}; !slices.Equal(got, want) {
		t.Errorf("rows left = %v, want %v: a row newer than the cutoff was pruned", got, want)
	}
}
