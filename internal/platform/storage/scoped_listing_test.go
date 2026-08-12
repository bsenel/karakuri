package storage_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/container"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	coretwin "github.com/bsenel/karakuri/internal/core/twin"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// The SQL is the security boundary here, so it is exercised against a real
// database rather than asserted about.
func newStore(t *testing.T) *storage.GORMStorage {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "listing.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.NewGORMStorage(db)
}

// seedTwins builds three twins: one in acme's eng team, one in globex's, and
// one in no container at all — the shape every twin had before Phase 17.
func seedTwins(t *testing.T, s *storage.GORMStorage) {
	t.Helper()
	ctx := context.Background()

	place := func(id string, labels ...string) {
		if err := s.SaveTwin(ctx, coretwin.DigitalTwin{ID: id, Name: id, Kind: coretwin.KindPerson}); err != nil {
			t.Fatalf("save twin %q: %v", id, err)
		}
		if len(labels) == 0 {
			return
		}
		if err := s.PutResourceScopes(ctx, containerScopes("twin", id, labels)); err != nil {
			t.Fatalf("scope twin %q: %v", id, err)
		}
	}
	place("acme-twin", "team:t_acme", "org:o_acme")
	place("globex-twin", "team:t_globex", "org:o_globex")
	place("loose-twin")
}

func listTwinIDs(t *testing.T, s *storage.GORMStorage, f storage.TwinFilter) []string {
	t.Helper()
	twins, err := s.ListTwins(context.Background(), f)
	if err != nil {
		t.Fatalf("list twins: %v", err)
	}
	ids := make([]string, 0, len(twins))
	for _, tw := range twins {
		ids = append(ids, tw.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestListTwinsByScope(t *testing.T) {
	s := newStore(t)
	seedTwins(t, s)

	cases := []struct {
		name   string
		filter storage.TwinFilter
		want   []string
	}{
		{
			name:   "no filter returns everything",
			filter: storage.TwinFilter{},
			want:   []string{"acme-twin", "globex-twin", "loose-twin"},
		},
		{
			name:   "one org",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{Labels: []string{"org:o_acme"}}},
			want:   []string{"acme-twin"},
		},
		{
			name:   "one team",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{Labels: []string{"team:t_globex"}}},
			want:   []string{"globex-twin"},
		},
		{
			// Which a path model cannot express: a set of unrelated subtrees in
			// one indexed query.
			name:   "several containers at once",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{Labels: []string{"org:o_acme", "team:t_globex"}}},
			want:   []string{"acme-twin", "globex-twin"},
		},
		{
			name:   "a twin named directly",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{IDs: []string{"loose-twin"}}},
			want:   []string{"loose-twin"},
		},
		{
			name: "identity and containment together",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{
				IDs: []string{"loose-twin"}, Labels: []string{"org:o_acme"},
			}},
			want: []string{"acme-twin", "loose-twin"},
		},
		{
			name:   "a whole kind of container",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{LabelPrefixes: []string{"org:"}}},
			want:   []string{"acme-twin", "globex-twin"},
		},
		{
			// The security-relevant one: an empty selector matches nothing. If
			// this ever returned every row, a principal with no grants would
			// see the whole database.
			name:   "an empty selector matches nothing",
			filter: storage.TwinFilter{Visible: &storage.ScopeSelector{}},
			want:   []string{},
		},
		{
			name: "a deny subtracts",
			filter: storage.TwinFilter{
				Visible: &storage.ScopeSelector{LabelPrefixes: []string{"org:"}},
				Hidden:  storage.ScopeSelector{Labels: []string{"org:o_globex"}},
			},
			want: []string{"acme-twin"},
		},
		{
			name:   "a deny with no visibility filter still subtracts",
			filter: storage.TwinFilter{Hidden: storage.ScopeSelector{Labels: []string{"org:o_acme"}}},
			want:   []string{"globex-twin", "loose-twin"},
		},
		{
			name: "scope filtering composes with the ordinary filters",
			filter: storage.TwinFilter{
				Kind:    string(coretwin.KindPerson),
				Visible: &storage.ScopeSelector{Labels: []string{"org:o_acme"}},
			},
			want: []string{"acme-twin"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := listTwinIDs(t, s, tc.filter); !slices.Equal(got, tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListObjectivesByScope(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	place := func(id, twinID string, labels ...string) {
		obj := coreobjective.Objective{
			ID: coreobjective.ObjectiveID(id), Title: id, TwinID: twinID,
			Status: coreobjective.StatusPending,
		}
		if err := s.SaveObjective(ctx, obj); err != nil {
			t.Fatalf("save objective %q: %v", id, err)
		}
		if len(labels) > 0 {
			if err := s.PutResourceScopes(ctx, containerScopes("objective", id, labels)); err != nil {
				t.Fatalf("scope objective %q: %v", id, err)
			}
		}
	}
	place("acme-obj", "acme-twin", "team:t_acme", "org:o_acme")
	place("globex-obj", "globex-twin", "team:t_globex", "org:o_globex")

	ids := func(f storage.ObjectiveFilter) []string {
		objs, err := s.ListObjectives(ctx, f)
		if err != nil {
			t.Fatalf("list objectives: %v", err)
		}
		out := make([]string, 0, len(objs))
		for _, o := range objs {
			out = append(out, string(o.ID))
		}
		slices.Sort(out)
		return out
	}

	if got := ids(storage.ObjectiveFilter{}); !slices.Equal(got, []string{"acme-obj", "globex-obj"}) {
		t.Fatalf("unfiltered = %v", got)
	}
	got := ids(storage.ObjectiveFilter{Visible: &storage.ScopeSelector{Labels: []string{"org:o_acme"}}})
	if !slices.Equal(got, []string{"acme-obj"}) {
		t.Fatalf("acme = %v, want just the acme objective", got)
	}
	// The scope filter and the twin_id parameter narrow together rather than
	// one overriding the other.
	got = ids(storage.ObjectiveFilter{
		TwinID:  "globex-twin",
		Visible: &storage.ScopeSelector{Labels: []string{"org:o_acme"}},
	})
	if len(got) != 0 {
		t.Fatalf("crossed filters = %v, want nothing", got)
	}
	if got := ids(storage.ObjectiveFilter{Visible: &storage.ScopeSelector{}}); len(got) != 0 {
		t.Fatalf("empty selector = %v, want nothing", got)
	}
}

// containerScopes builds the row set for a resource whose labels are all
// declared rather than inherited. The distinction does not matter to a listing
// — it matches on the label either way — so these tests do not model a tree.
func containerScopes(resourceType, id string, labels []string) container.ResourceScopes {
	return container.ResourceScopes{
		ResourceType: resourceType, ResourceID: id,
		Direct: labels, All: labels,
	}
}
