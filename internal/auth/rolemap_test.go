package auth_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	corecontainer "github.com/bsenel/karakuri/internal/core/container"
	featurecontainer "github.com/bsenel/karakuri/internal/feature/container"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func containerService(t *testing.T) *featurecontainer.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "containers.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return featurecontainer.NewService(storage.NewGORMStorage(db))
}

func create(t *testing.T, s *featurecontainer.Service, kind corecontainer.Kind, name, parent string) corecontainer.Container {
	t.Helper()
	c, err := s.Create(context.Background(), featurecontainer.CreateRequest{Kind: kind, Name: name, ParentID: parent})
	if err != nil {
		t.Fatalf("create %s %q: %v", kind, name, err)
	}
	return c
}

// The two-tenant case, resolved from the file an operator actually writes: both
// orgs have a team called "eng", and the two mappings must come out as
// different labels.
func TestBuildRoleMapResolvesTeamsPerOrg(t *testing.T) {
	svc := containerService(t)
	acme := create(t, svc, corecontainer.KindOrg, "acme", "")
	globex := create(t, svc, corecontainer.KindOrg, "globex", "")
	acmeEng := create(t, svc, corecontainer.KindTeam, "eng", acme.ID)
	globexEng := create(t, svc, corecontainer.KindTeam, "eng", globex.ID)

	got, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
		Groups: map[string][]config.AuthRoleGrantConfig{
			"acme-engineers":   {{Role: "operator", Org: "acme", Team: "eng"}},
			"globex-engineers": {{Role: "operator", Org: "globex", Team: "eng"}},
			"acme-admins":      {{Role: "admin", Org: "acme"}},
			"everyone":         {{Role: "viewer"}},
		},
	}, svc)
	if err != nil {
		t.Fatalf("BuildRoleMap: %v", err)
	}

	want := map[string]extauth.RoleGrant{
		"acme-engineers":   {Role: "operator", Scope: acmeEng.Label()},
		"globex-engineers": {Role: "operator", Scope: globexEng.Label()},
		"acme-admins":      {Role: "admin", Scope: acme.Label()},
		"everyone":         {Role: "viewer", Scope: "*"},
	}
	for group, expected := range want {
		grants := got.Groups[group]
		if len(grants) != 1 || grants[0] != expected {
			t.Errorf("group %q = %v, want [%v]", group, grants, expected)
		}
	}
	if got.Groups["acme-engineers"][0].Scope == got.Groups["globex-engineers"][0].Scope {
		t.Fatal("two teams called eng resolved to the same scope")
	}
}

func TestBuildRoleMapResolvesProjectsAndDefaults(t *testing.T) {
	svc := containerService(t)
	delta := create(t, svc, corecontainer.KindProject, "delta", "")

	got, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
		Default: []config.AuthRoleGrantConfig{{Role: "viewer", Project: "delta"}},
	}, svc)
	if err != nil {
		t.Fatalf("BuildRoleMap: %v", err)
	}
	want := []extauth.RoleGrant{{Role: "viewer", Scope: delta.Label()}}
	if !slices.Equal(got.Default, want) {
		t.Fatalf("Default = %v, want %v", got.Default, want)
	}
}

// Nothing configured is the common case and must stay free of surprises: no
// groups, no default, no container lookups.
func TestBuildRoleMapEmpty(t *testing.T) {
	got, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{}, nil)
	if err != nil {
		t.Fatalf("BuildRoleMap: %v", err)
	}
	if len(got.Groups) != 0 || len(got.Default) != 0 {
		t.Fatalf("map = %+v, want empty", got)
	}
}

// An unscoped grant resolves without a container tree at all, so a deployment
// that never creates an org keeps working exactly as it did.
func TestBuildRoleMapWithoutAContainerTree(t *testing.T) {
	got, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
		Groups: map[string][]config.AuthRoleGrantConfig{"eng": {{Role: "operator"}}},
	}, nil)
	if err != nil {
		t.Fatalf("BuildRoleMap: %v", err)
	}
	if got.Groups["eng"][0].Scope != "*" {
		t.Fatalf("scope = %q, want *", got.Groups["eng"][0].Scope)
	}

	// But naming a container with no tree is a configuration error, not a
	// silent grant over everything.
	if _, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
		Groups: map[string][]config.AuthRoleGrantConfig{"eng": {{Role: "operator", Org: "acme"}}},
	}, nil); err == nil {
		t.Fatal("a grant naming an org resolved against no tree")
	}
}

// Every one of these is a typo an operator should read about at startup rather
// than discover when somebody cannot see their twins.
func TestBuildRoleMapRejects(t *testing.T) {
	svc := containerService(t)
	acme := create(t, svc, corecontainer.KindOrg, "acme", "")
	create(t, svc, corecontainer.KindTeam, "eng", acme.ID)
	// Two sub-teams of different parents may share a name — legal in the tree,
	// but not addressable from configuration.
	platform := create(t, svc, corecontainer.KindTeam, "platform", acme.ID)
	create(t, svc, corecontainer.KindTeam, "shared", platform.ID)
	ops := create(t, svc, corecontainer.KindTeam, "ops", acme.ID)
	create(t, svc, corecontainer.KindTeam, "shared", ops.ID)

	cases := []struct {
		name  string
		grant config.AuthRoleGrantConfig
		want  string
	}{
		{"no role", config.AuthRoleGrantConfig{Org: "acme"}, "no role"},
		{"unknown org", config.AuthRoleGrantConfig{Role: "admin", Org: "widgets"}, `no org called "widgets"`},
		{"unknown team", config.AuthRoleGrantConfig{Role: "operator", Org: "acme", Team: "sales"}, `no team "sales"`},
		{"unknown project", config.AuthRoleGrantConfig{Role: "viewer", Project: "gamma"}, `no project called "gamma"`},
		{"team without its org", config.AuthRoleGrantConfig{Role: "operator", Team: "eng"}, "needs its org"},
		{"project qualified by an org", config.AuthRoleGrantConfig{Role: "viewer", Project: "delta", Org: "acme"}, "cannot also name"},
		{"ambiguous team", config.AuthRoleGrantConfig{Role: "operator", Org: "acme", Team: "shared"}, "teams called"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
				Groups: map[string][]config.AuthRoleGrantConfig{"eng": {tc.grant}},
			}, svc)
			if err == nil {
				t.Fatal("resolved a grant that names nothing that exists")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
			// The group is named, because "which line of my config" is the
			// first thing an operator needs to know.
			if !strings.Contains(err.Error(), `group "eng"`) {
				t.Errorf("err = %v, want the group named", err)
			}
		})
	}

	// The same failure in the default block names the default block.
	_, err := karakuriauth.BuildRoleMap(context.Background(), config.AuthRoleMapConfig{
		Default: []config.AuthRoleGrantConfig{{Role: "viewer", Org: "widgets"}},
	}, svc)
	if err == nil || !strings.Contains(err.Error(), "default") {
		t.Fatalf("err = %v, want the default block named", err)
	}
}
