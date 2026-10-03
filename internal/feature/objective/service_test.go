package objective

import (
	"context"
	"path/filepath"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(storage.NewGORMStorage(db))
}

// The template an objective was created from is kept on the objective, and
// survives the store, so it can be named later by whoever asks what shaped a
// decision.
func TestCreateRecordsTheTemplateItUsed(t *testing.T) {
	svc := newTestService(t)
	svc.RegisterTemplate(coreobjective.Template{ID: "software.bugfix"})

	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "fix it", Domain: "software", TemplateID: "software.bugfix",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if o.TemplateID != "software.bugfix" {
		t.Errorf("TemplateID = %q, want software.bugfix", o.TemplateID)
	}
	got, err := svc.Get(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TemplateID != "software.bugfix" {
		t.Errorf("stored TemplateID = %q, want software.bugfix", got.TemplateID)
	}
}

// A template that was asked for but is not registered shaped nothing, so the
// objective does not claim it.
func TestCreateLeavesTemplateIDEmptyWhenNoTemplateWasUsed(t *testing.T) {
	svc := newTestService(t)

	for name, req := range map[string]CreateRequest{
		"no template":      {Title: "plain", Domain: "software"},
		"unknown template": {Title: "plain", Domain: "software", TemplateID: "nope"},
	} {
		o, err := svc.Create(context.Background(), req)
		if err != nil {
			t.Fatalf("%s: create: %v", name, err)
		}
		if o.TemplateID != "" {
			t.Errorf("%s: TemplateID = %q, want empty", name, o.TemplateID)
		}
	}
}
