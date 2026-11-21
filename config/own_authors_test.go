package config

import "testing"

// Own authors (Phase 40 step 12a) load as written, and none at all is valid.
func TestLoad_OwnAuthorsParsed(t *testing.T) {
	cfg := loadYAML(t, "own_authors:\n  karakuri: [karakuri-bot]\n  owners: [bsenel, co-owner]\n")
	got := cfg.OwnAuthors
	if len(got.Karakuri) != 1 || got.Karakuri[0] != "karakuri-bot" {
		t.Errorf("OwnAuthors.Karakuri = %v, want [karakuri-bot]", got.Karakuri)
	}
	if len(got.Owners) != 2 || got.Owners[0] != "bsenel" || got.Owners[1] != "co-owner" {
		t.Errorf("OwnAuthors.Owners = %v, want [bsenel co-owner]", got.Owners)
	}
}

func TestLoad_OwnAuthorsEmptyIsValid(t *testing.T) {
	if got := loadYAML(t, "executor: local\n").OwnAuthors; len(got.Karakuri) != 0 || len(got.Owners) != 0 {
		t.Errorf("absent own_authors = %+v, want both lists empty", got)
	}
	t.Setenv("GITHUB_TOKEN", "preexisting-token")
	cfg, err := Load("default.yaml")
	if err != nil {
		t.Fatalf("Load default.yaml: %v", err)
	}
	if got := cfg.OwnAuthors; len(got.Karakuri) != 0 || len(got.Owners) != 0 {
		t.Errorf("default.yaml own_authors = %+v, want both lists empty", got)
	}
}
