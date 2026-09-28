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
