package storage_test

import (
	"context"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

func TestObjectiveTemplateIDRoundTrips(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.SaveObjective(ctx, coreobjective.Objective{
		ID: "o-tmpl", Title: "t", Domain: "software", TemplateID: "software.bugfix",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.GetObjective(ctx, "o-tmpl")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TemplateID != "software.bugfix" {
		t.Errorf("TemplateID = %q, want software.bugfix", got.TemplateID)
	}
}

// A row written before the column existed names no template, and reads back
// that way.
func TestObjectiveWrittenWithoutTemplateIDReadsBackEmpty(t *testing.T) {
	store, db := newStoreWithDB(t)

	if err := db.Exec(
		`INSERT INTO objectives (id, title, domain, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"o-old", "older row", "software",
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := store.GetObjective(context.Background(), "o-old")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TemplateID != "" {
		t.Errorf("TemplateID = %q, want empty", got.TemplateID)
	}
}
