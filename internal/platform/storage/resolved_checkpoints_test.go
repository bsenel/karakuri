package storage_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// The calibration report is only as honest as this listing: a pending
// checkpoint in it would be scored against a human verdict nobody gave.
func TestListResolvedCheckpoints(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	save := func(id, twin string) {
		t.Helper()
		err := s.SaveCheckpoint(ctx, checkpoint.Checkpoint{
			ID: id, ObjectiveID: coreobjective.ObjectiveID("obj-" + id), TwinID: twin,
			Summary: "summary " + id,
			Options: []string{"approve", "reject", "modify"},
			Actions: []checkpoint.Action{{
				CapabilityID: "vcs.open_pr",
				Params:       map[string]any{"title": "pr for " + id},
				Reason:       "because " + id,
			}},
			Status: checkpoint.StatusPending,
		})
		if err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	resolve := func(id, choice string) {
		t.Helper()
		if err := s.ResolveCheckpoint(ctx, id, checkpoint.Decision{Choice: choice, Note: "note " + id}); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
	}

	save("a1", "twin-a")
	save("a2", "twin-a")
	save("b1", "twin-b")
	save("pending", "twin-a")
	resolve("a1", "approve")
	resolve("a2", "reject")
	resolve("b1", "modify")

	list := func(f storage.ResolvedCheckpointFilter) []checkpoint.Checkpoint {
		t.Helper()
		cps, err := s.ListResolvedCheckpoints(ctx, f)
		if err != nil {
			t.Fatalf("list %+v: %v", f, err)
		}
		return cps
	}
	ids := func(cps []checkpoint.Checkpoint) []string {
		out := make([]string, 0, len(cps))
		for _, c := range cps {
			out = append(out, c.ID)
		}
		slices.Sort(out)
		return out
	}

	all := list(storage.ResolvedCheckpointFilter{})
	if got, want := ids(all), []string{"a1", "a2", "b1"}; !slices.Equal(got, want) {
		t.Fatalf("empty filter = %v, want %v", got, want)
	}
	wantChoice := map[string]string{"a1": "approve", "a2": "reject", "b1": "modify"}
	for _, c := range all {
		if c.Decision == nil {
			t.Fatalf("%s: Decision not round-tripped", c.ID)
		}
		if c.Decision.Choice != wantChoice[c.ID] {
			t.Errorf("%s: Choice = %q, want %q", c.ID, c.Decision.Choice, wantChoice[c.ID])
		}
		if c.Decision.Note != "note "+c.ID {
			t.Errorf("%s: Note = %q", c.ID, c.Decision.Note)
		}
		if len(c.Actions) != 1 {
			t.Fatalf("%s: Actions = %+v, want 1", c.ID, c.Actions)
		}
		a := c.Actions[0]
		if a.CapabilityID != "vcs.open_pr" || a.Reason != "because "+c.ID || a.Params["title"] != "pr for "+c.ID {
			t.Errorf("%s: Action not round-tripped: %+v", c.ID, a)
		}
	}

	if got, want := ids(list(storage.ResolvedCheckpointFilter{TwinID: "twin-a"})), []string{"a1", "a2"}; !slices.Equal(got, want) {
		t.Errorf("TwinID=twin-a = %v, want %v", got, want)
	}

	now := time.Now()
	if got := list(storage.ResolvedCheckpointFilter{Since: now.Add(time.Hour)}); len(got) != 0 {
		t.Errorf("Since=now+1h = %v, want none", ids(got))
	}
	if got := list(storage.ResolvedCheckpointFilter{Until: now.Add(-time.Hour)}); len(got) != 0 {
		t.Errorf("Until=now-1h = %v, want none", ids(got))
	}
}

// --limit is the operator's spend bound on a calibration, which costs one
// model call per checkpoint listed here. The cap keeps the newest decisions
// and still hands them back oldest-first.
func TestListResolvedCheckpoints_Limit(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Resolved in this order, a few milliseconds apart so resolved_at orders
	// them; marks[i] is an instant just before the i-th resolution.
	seed := []struct{ id, twin string }{
		{"c1", "twin-a"}, {"c2", "twin-b"}, {"c3", "twin-a"}, {"c4", "twin-b"}, {"c5", "twin-a"},
	}
	marks := make([]time.Time, len(seed))
	for i, c := range seed {
		err := s.SaveCheckpoint(ctx, checkpoint.Checkpoint{
			ID: c.id, ObjectiveID: coreobjective.ObjectiveID("obj-" + c.id), TwinID: c.twin,
			Summary: "summary " + c.id,
			Options: []string{"approve", "reject", "modify"},
			Status:  checkpoint.StatusPending,
		})
		if err != nil {
			t.Fatalf("save %s: %v", c.id, err)
		}
		time.Sleep(5 * time.Millisecond)
		marks[i] = time.Now()
		time.Sleep(5 * time.Millisecond)
		if err := s.ResolveCheckpoint(ctx, c.id, checkpoint.Decision{Choice: "approve"}); err != nil {
			t.Fatalf("resolve %s: %v", c.id, err)
		}
	}

	// In result order, unsorted: the order is part of what is asserted.
	list := func(f storage.ResolvedCheckpointFilter) []string {
		t.Helper()
		cps, err := s.ListResolvedCheckpoints(ctx, f)
		if err != nil {
			t.Fatalf("list %+v: %v", f, err)
		}
		out := make([]string, 0, len(cps))
		for _, c := range cps {
			out = append(out, c.ID)
		}
		return out
	}

	if got, want := list(storage.ResolvedCheckpointFilter{Limit: 2}), []string{"c4", "c5"}; !slices.Equal(got, want) {
		t.Errorf("Limit=2 = %v, want %v (the most recently resolved, oldest first)", got, want)
	}
	if got, want := list(storage.ResolvedCheckpointFilter{}), []string{"c1", "c2", "c3", "c4", "c5"}; !slices.Equal(got, want) {
		t.Errorf("Limit=0 = %v, want %v", got, want)
	}
	if got, want := list(storage.ResolvedCheckpointFilter{Limit: 10}), []string{"c1", "c2", "c3", "c4", "c5"}; !slices.Equal(got, want) {
		t.Errorf("Limit=10 = %v, want %v", got, want)
	}
	// The cap applies within the twin's matches, not before the twin filter.
	if got, want := list(storage.ResolvedCheckpointFilter{TwinID: "twin-a", Limit: 2}), []string{"c3", "c5"}; !slices.Equal(got, want) {
		t.Errorf("TwinID=twin-a Limit=2 = %v, want %v", got, want)
	}
	if got, want := list(storage.ResolvedCheckpointFilter{TwinID: "twin-b", Limit: 1}), []string{"c4"}; !slices.Equal(got, want) {
		t.Errorf("TwinID=twin-b Limit=1 = %v, want %v", got, want)
	}
	// And within the window: c2..c4 are in it, and the newest two of those are kept.
	window := storage.ResolvedCheckpointFilter{Since: marks[1], Until: marks[4], Limit: 2}
	if got, want := list(window), []string{"c3", "c4"}; !slices.Equal(got, want) {
		t.Errorf("Since/Until Limit=2 = %v, want %v", got, want)
	}
	if got, want := list(storage.ResolvedCheckpointFilter{Since: marks[3], Limit: 1}), []string{"c5"}; !slices.Equal(got, want) {
		t.Errorf("Since Limit=1 = %v, want %v", got, want)
	}
	if got, want := list(storage.ResolvedCheckpointFilter{Until: marks[2], Limit: 1}), []string{"c2"}; !slices.Equal(got, want) {
		t.Errorf("Until Limit=1 = %v, want %v", got, want)
	}
}
