package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// forge answers the three reads the reader makes and nothing else; the
// embedded interface is nil, so any other call would panic.
type forge struct {
	versioncontrol.VersionControlAdapter
	active  bool
	prs     []versioncontrol.PRSummary
	reviews map[string][]versioncontrol.Review
	err     error
}

func (f forge) Active() bool { return f.active }
func (f forge) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	return f.prs, f.err
}
func (f forge) ListReviews(_ context.Context, _, id string) ([]versioncontrol.Review, error) {
	return f.reviews[id], f.err
}

func seedStreams(t *testing.T) storage.StorageAdapter {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	ctx := context.Background()
	if err := store.SaveTwin(ctx, twin.DigitalTwin{ID: "twin-1", Name: "t", Kind: twin.KindPerson, Domain: "software"}); err != nil {
		t.Fatalf("seed twin: %v", err)
	}
	for id, tmpl := range map[string]string{
		"obj-delivery": "software.objective.roadmap_delivery",
		"obj-ux":       "software.objective.ux_improvement",
	} {
		err := store.SaveObjective(ctx, objective.Objective{
			ID: objective.ObjectiveID(id), Title: id, Domain: "software", TwinID: "twin-1",
			Mode: objective.ModeStanding, Status: objective.StatusConverged, TemplateID: tmpl,
		})
		if err != nil {
			t.Fatalf("seed objective: %v", err)
		}
	}
	return store
}

func registryWith(f forge) *tools.Registry {
	reg := &tools.Registry{}
	reg.VC.Set("github", "github", f)
	return reg
}

// Phase 40 step 14: a pull request is its stream's by the branch rule the git
// observation uses, and carries the latest review that says something.
func TestStreamPullRequestsAreTaggedByStreamWithTheLatestReview(t *testing.T) {
	store := seedStreams(t)
	f := forge{
		active: true,
		prs: []versioncontrol.PRSummary{
			{ID: "10", Number: 10, Title: "delivery", HeadBranch: "karakuri/phase-40-step-14-x", HeadSHA: "h2"},
			{ID: "11", Number: 11, Title: "ux", HeadBranch: "karakuri/ux-copy", HeadSHA: "u1", Draft: true},
			{ID: "12", Number: 12, Title: "a human's", HeadBranch: "feature/mine", HeadSHA: "m1"},
			{ID: "13", Number: 13, Title: "unreviewed", HeadBranch: "karakuri/eb-007", HeadSHA: "e1"},
		},
		reviews: map[string][]versioncontrol.Review{
			// The newest review that says something wins; pending says nothing.
			"10": {{State: "changes_requested", CommitID: "h1"}, {State: "approved", CommitID: "h2"}, {State: "pending", CommitID: "h2"}},
		},
	}
	got, err := streamPullRequests{store: store, tools: registryWith(f)}.StreamPullRequests(context.Background(), "twin-1")
	if err != nil {
		t.Fatalf("StreamPullRequests: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d pull requests, want 3 (the human's is no stream's): %+v", len(got), got)
	}
	want := map[int]struct {
		obj, review string
		head, draft bool
	}{
		10: {"obj-delivery", "approved", true, false},
		11: {"obj-ux", "none", false, true},
		13: {"obj-delivery", "none", false, false},
	}
	for _, p := range got {
		w := want[p.Number]
		if p.ObjectiveID != w.obj || p.Review != w.review || p.ReviewedHead != w.head || p.Draft != w.draft || p.State != "open" {
			t.Errorf("#%d = %+v, want %+v", p.Number, p, w)
		}
	}
}

// Blind, not quiet: a forge that is not wired or cannot be read is an error.
func TestStreamPullRequestsErrorWhenTheForgeCannotBeRead(t *testing.T) {
	store := seedStreams(t)
	for name, f := range map[string]forge{
		"inactive forge":     {active: false},
		"failing list":       {active: true, err: errors.New("502")},
		"unreadable reviews": {active: true, prs: []versioncontrol.PRSummary{{ID: "1", Number: 1, HeadBranch: "karakuri/ux-a"}}, err: errors.New("403")},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := streamPullRequests{store: store, tools: registryWith(f)}.StreamPullRequests(context.Background(), "twin-1")
			if err == nil {
				t.Fatalf("got %+v and no error, want an error", got)
			}
		})
	}
}
