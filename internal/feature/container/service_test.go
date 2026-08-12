package container_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/auth"
	corecontainer "github.com/bsenel/karakuri/internal/core/container"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/feature/container"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func newService(t *testing.T) *container.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "containers.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return container.NewService(storage.NewGORMStorage(db))
}

func mustCreate(t *testing.T, s *container.Service, kind corecontainer.Kind, name, parent string) corecontainer.Container {
	t.Helper()
	c, err := s.Create(context.Background(), container.CreateRequest{Kind: kind, Name: name, ParentID: parent})
	if err != nil {
		t.Fatalf("create %s %q: %v", kind, name, err)
	}
	return c
}

// The case the whole design exists to survive: two tenants each with a team
// called "Engineering". Both must be creatable, and neither one's label may
// reach the other's resources.
func TestTwoOrgsMayEachHaveATeamCalledEngineering(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	acmeEng := mustCreate(t, s, corecontainer.KindTeam, "Engineering", acme.ID)
	globexEng := mustCreate(t, s, corecontainer.KindTeam, "Engineering", globex.ID)

	if acmeEng.ID == globexEng.ID {
		t.Fatal("two teams with the same name got the same id")
	}

	if err := s.SetResourceContainers(ctx, "twin", "twin-a", []string{acmeEng.ID}); err != nil {
		t.Fatalf("scope acme twin: %v", err)
	}
	if err := s.SetResourceContainers(ctx, "twin", "twin-g", []string{globexEng.ID}); err != nil {
		t.Fatalf("scope globex twin: %v", err)
	}

	acmeScopes, err := s.ScopesOf(ctx, "twin", "twin-a")
	if err != nil {
		t.Fatalf("scopes of acme twin: %v", err)
	}
	if slices.Contains(acmeScopes, globexEng.Label()) || slices.Contains(acmeScopes, globex.Label()) {
		t.Fatalf("acme twin carries a globex label: %v", acmeScopes)
	}

	// And the labels are what authorization actually matches on: a binding
	// scoped to acme's Engineering must not cover globex's twin.
	globexTwin := auth.Resource("twin", "twin-g")
	globexScopes, err := s.ScopesOf(ctx, "twin", "twin-g")
	if err != nil {
		t.Fatalf("scopes of globex twin: %v", err)
	}
	globexTwin = globexTwin.WithScopes(globexScopes...)
	if globexTwin.InScope(acmeEng.Label()) {
		t.Fatalf("acme grant reaches globex twin: %v", globexScopes)
	}
	if !globexTwin.InScope(globex.Label()) {
		t.Fatalf("globex org grant does not reach its own twin: %v", globexScopes)
	}
}

func TestClosureIsNearestFirst(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	holding := mustCreate(t, s, corecontainer.KindOrg, "holding", "")
	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", holding.ID)
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	got, err := s.Closure(ctx, eng.ID)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	want := []string{eng.Label(), acme.Label(), holding.Label()}
	if !slices.Equal(got, want) {
		t.Fatalf("closure = %v, want %v", got, want)
	}
}

func TestScopeLabelsAreValidBindingScopes(t *testing.T) {
	s := newService(t)
	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	labels, err := s.Closure(context.Background(), eng.ID)
	if err != nil {
		t.Fatalf("closure: %v", err)
	}
	if err := auth.ValidateScopes(labels); err != nil {
		t.Fatalf("closure is not usable as scopes: %v", err)
	}
}

// Container.Label and auth.ScopeLabel build the same string from opposite sides
// of a module boundary. They cannot be one function — this package is
// dependency-free domain — so the agreement is pinned here instead.
func TestLabelMatchesAuthScopeLabel(t *testing.T) {
	for _, kind := range []corecontainer.Kind{
		corecontainer.KindOrg, corecontainer.KindTeam, corecontainer.KindProject,
	} {
		c := corecontainer.Container{ID: "x_123", Kind: kind}
		if got, want := c.Label(), auth.ScopeLabel(string(kind), "x_123"); got != want {
			t.Fatalf("Label() = %q, auth.ScopeLabel = %q", got, want)
		}
	}
}

func TestNestingRules(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	delta := mustCreate(t, s, corecontainer.KindProject, "delta", "")

	// A sub-team is fine.
	mustCreate(t, s, corecontainer.KindTeam, "platform", eng.ID)

	cases := []struct {
		name string
		req  container.CreateRequest
	}{
		{"team at the root", container.CreateRequest{Kind: corecontainer.KindTeam, Name: "orphan"}},
		{"team inside a project", container.CreateRequest{Kind: corecontainer.KindTeam, Name: "t", ParentID: delta.ID}},
		{"org inside a team", container.CreateRequest{Kind: corecontainer.KindOrg, Name: "o", ParentID: eng.ID}},
		{"project inside an org", container.CreateRequest{Kind: corecontainer.KindProject, Name: "p", ParentID: acme.ID}},
		{"unknown kind", container.CreateRequest{Kind: "squad", Name: "s"}},
		{"no name", container.CreateRequest{Kind: corecontainer.KindOrg, Name: "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Create(ctx, tc.req); !errors.Is(err, coreerrors.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestNamesAreUniquePerParentOnly(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	// Same name, same parent: refused.
	_, err := s.Create(ctx, container.CreateRequest{Kind: corecontainer.KindTeam, Name: "eng", ParentID: acme.ID})
	if !errors.Is(err, container.ErrDuplicateName) {
		t.Fatalf("duplicate sibling err = %v, want ErrDuplicateName", err)
	}
	// Same name, different parent: fine. This is the multi-tenancy case.
	mustCreate(t, s, corecontainer.KindTeam, "eng", globex.ID)

	// Two roots may not share a name either.
	_, err = s.Create(ctx, container.CreateRequest{Kind: corecontainer.KindOrg, Name: "acme"})
	if !errors.Is(err, container.ErrDuplicateName) {
		t.Fatalf("duplicate root err = %v, want ErrDuplicateName", err)
	}
}

func TestRename(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	ops := mustCreate(t, s, corecontainer.KindTeam, "ops", acme.ID)

	renamed, err := s.Rename(ctx, eng.ID, "Engineering")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "Engineering" {
		t.Fatalf("name = %q", renamed.Name)
	}
	// The label is unchanged, which is the payoff of scoping on IDs: no binding
	// anywhere had to be rewritten.
	if renamed.Label() != eng.Label() {
		t.Fatalf("rename changed the label: %q → %q", eng.Label(), renamed.Label())
	}
	// The old name is free again.
	mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	if _, err := s.Rename(ctx, ops.ID, "Engineering"); !errors.Is(err, container.ErrDuplicateName) {
		t.Fatalf("rename onto a sibling err = %v, want ErrDuplicateName", err)
	}
	// Renaming to the name it already has is a no-op, not a conflict with
	// itself.
	if _, err := s.Rename(ctx, ops.ID, "ops"); err != nil {
		t.Fatalf("rename to same name: %v", err)
	}
	if _, err := s.Rename(ctx, ops.ID, "   "); !errors.Is(err, coreerrors.ErrInvalidInput) {
		t.Fatalf("rename to blank err = %v, want ErrInvalidInput", err)
	}
}

func TestDepthIsCapped(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	parent := ""
	var deepest corecontainer.Container
	for i := 0; i < corecontainer.MaxDepth; i++ {
		deepest = mustCreate(t, s, corecontainer.KindOrg, string(rune('a'+i)), parent)
		parent = deepest.ID
	}
	if _, err := s.Create(ctx, container.CreateRequest{
		Kind: corecontainer.KindOrg, Name: "one-too-deep", ParentID: deepest.ID,
	}); !errors.Is(err, container.ErrTooDeep) {
		t.Fatalf("err = %v, want ErrTooDeep", err)
	}
}

func TestReparentRejectsCyclesAndOverdeepSubtrees(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	root := mustCreate(t, s, corecontainer.KindOrg, "root", "")
	mid := mustCreate(t, s, corecontainer.KindOrg, "mid", root.ID)
	leaf := mustCreate(t, s, corecontainer.KindOrg, "leaf", mid.ID)

	if _, err := s.Reparent(ctx, root.ID, root.ID); !errors.Is(err, container.ErrCycle) {
		t.Fatalf("self-parent err = %v, want ErrCycle", err)
	}
	if _, err := s.Reparent(ctx, root.ID, leaf.ID); !errors.Is(err, container.ErrCycle) {
		t.Fatalf("parent-under-descendant err = %v, want ErrCycle", err)
	}

	// Build a chain deep enough that moving the 3-level subtree under it would
	// exceed the cap.
	deep := ""
	for i := 0; i < corecontainer.MaxDepth-2; i++ {
		c := mustCreate(t, s, corecontainer.KindOrg, "d"+string(rune('a'+i)), deep)
		deep = c.ID
	}
	if _, err := s.Reparent(ctx, root.ID, deep); !errors.Is(err, container.ErrTooDeep) {
		t.Fatalf("overdeep subtree err = %v, want ErrTooDeep", err)
	}
}

func TestReparentRebuildsTheClosureBeneathIt(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	squad := mustCreate(t, s, corecontainer.KindTeam, "squad", eng.ID)

	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{squad.ID}); err != nil {
		t.Fatalf("scope twin: %v", err)
	}
	before, err := s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if !slices.Contains(before, acme.Label()) {
		t.Fatalf("twin does not inherit its org: %v", before)
	}

	if _, err := s.Reparent(ctx, eng.ID, globex.ID); err != nil {
		t.Fatalf("reparent: %v", err)
	}

	after, err := s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes after reparent: %v", err)
	}
	if slices.Contains(after, acme.Label()) {
		t.Fatalf("twin still visible to the old org: %v", after)
	}
	if !slices.Contains(after, globex.Label()) {
		t.Fatalf("twin not visible to the new org: %v", after)
	}
	// Its declared membership is untouched — only what was derived moved.
	if !slices.Contains(after, squad.Label()) {
		t.Fatalf("twin lost its own team: %v", after)
	}

	// Reparenting to the parent it already has is a no-op.
	if _, err := s.Reparent(ctx, eng.ID, globex.ID); err != nil {
		t.Fatalf("no-op reparent: %v", err)
	}
}

func TestReparentToTheRoot(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	holding := mustCreate(t, s, corecontainer.KindOrg, "holding", "")
	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", holding.ID)
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{eng.ID}); err != nil {
		t.Fatalf("scope: %v", err)
	}
	// A subsidiary spun out: acme becomes its own root, and everything beneath
	// it stops being visible to the former parent.
	if _, err := s.Reparent(ctx, acme.ID, ""); err != nil {
		t.Fatalf("reparent to root: %v", err)
	}
	scopes, err := s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if slices.Contains(scopes, holding.Label()) {
		t.Fatalf("twin still visible to the former parent org: %v", scopes)
	}
	if !slices.Contains(scopes, acme.Label()) {
		t.Fatalf("twin lost its own org: %v", scopes)
	}

	// A team may not become a root — it would belong to no tenant.
	if _, err := s.Reparent(ctx, eng.ID, ""); !errors.Is(err, coreerrors.ErrInvalidInput) {
		t.Fatalf("team to root err = %v, want ErrInvalidInput", err)
	}
}

func TestMultiHoming(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	delta := mustCreate(t, s, corecontainer.KindProject, "delta", "")

	// One twin, in acme's team and in a project that spans both tenants. A path
	// model cannot express this at all.
	if err := s.SetResourceContainers(ctx, "twin", "shared", []string{eng.ID, delta.ID}); err != nil {
		t.Fatalf("scope twin: %v", err)
	}
	scopes, err := s.ScopesOf(ctx, "twin", "shared")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	ref := auth.Resource("twin", "shared").WithScopes(scopes...)

	for _, label := range []string{eng.Label(), acme.Label(), delta.Label()} {
		if !ref.InScope(label) {
			t.Fatalf("%s does not reach the shared twin: %v", label, scopes)
		}
	}
	// Sharing is per-resource: being in the project does not put the twin in
	// globex.
	if ref.InScope(globex.Label()) {
		t.Fatalf("project membership leaked the twin into globex: %v", scopes)
	}
}

func TestSetResourceContainersReplaces(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	ops := mustCreate(t, s, corecontainer.KindTeam, "ops", acme.ID)

	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{eng.ID}); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{ops.ID}); err != nil {
		t.Fatalf("rescope: %v", err)
	}
	scopes, err := s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if slices.Contains(scopes, eng.Label()) {
		t.Fatalf("the old team survived a replace: %v", scopes)
	}
	if !slices.Contains(scopes, ops.Label()) {
		t.Fatalf("the new team is missing: %v", scopes)
	}

	// Leaving the tree entirely returns the resource to how everything behaved
	// before Phase 17: no labels, so only its own id and the wildcards match.
	if err := s.SetResourceContainers(ctx, "twin", "t1", nil); err != nil {
		t.Fatalf("unscope: %v", err)
	}
	scopes, err = s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if len(scopes) != 0 {
		t.Fatalf("scopes = %v, want none", scopes)
	}
	ref := auth.Resource("twin", "t1").WithScopes(scopes...)
	if !ref.InScope("twin:t1") || !ref.InScope("*") || ref.InScope(acme.Label()) {
		t.Fatal("an unscoped twin does not match what it matched before containers existed")
	}
}

func TestSetResourceContainersRejectsAnUnnamedResource(t *testing.T) {
	s := newService(t)
	if err := s.SetResourceContainers(context.Background(), "", "t1", nil); !errors.Is(err, coreerrors.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if err := s.SetResourceContainers(context.Background(), "twin", "", nil); !errors.Is(err, coreerrors.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUnknownContainersAreNotFound(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	if _, err := s.Get(ctx, "nope"); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("get err = %v, want ErrContainerNotFound", err)
	}
	if _, err := s.Closure(ctx, "nope"); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("closure err = %v, want ErrContainerNotFound", err)
	}
	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{"nope"}); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("scope err = %v, want ErrContainerNotFound", err)
	}
	if _, err := s.Create(ctx, container.CreateRequest{
		Kind: corecontainer.KindTeam, Name: "eng", ParentID: "nope",
	}); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("create err = %v, want ErrContainerNotFound", err)
	}
	if _, err := s.Reparent(ctx, "nope", ""); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("reparent err = %v, want ErrContainerNotFound", err)
	}
	if _, err := s.Rename(ctx, "nope", "x"); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("rename err = %v, want ErrContainerNotFound", err)
	}
	if err := s.Delete(ctx, "nope"); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("delete err = %v, want ErrContainerNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	delta := mustCreate(t, s, corecontainer.KindProject, "delta", "")

	if err := s.Delete(ctx, acme.ID); !errors.Is(err, container.ErrNotEmpty) {
		t.Fatalf("delete non-empty err = %v, want ErrNotEmpty", err)
	}

	// A twin in both the team and the project keeps the project when the team
	// goes away.
	if err := s.SetResourceContainers(ctx, "twin", "t1", []string{eng.ID, delta.ID}); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if err := s.Delete(ctx, eng.ID); err != nil {
		t.Fatalf("delete leaf: %v", err)
	}
	scopes, err := s.ScopesOf(ctx, "twin", "t1")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if slices.Contains(scopes, eng.Label()) || slices.Contains(scopes, acme.Label()) {
		t.Fatalf("twin still carries the deleted team or what it inherited through it: %v", scopes)
	}
	if !slices.Contains(scopes, delta.Label()) {
		t.Fatalf("twin lost its project: %v", scopes)
	}
	if _, err := s.Get(ctx, eng.ID); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("container survived delete: %v", err)
	}
}

func TestList(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)

	roots, err := s.List(ctx, corecontainer.Filter{RootsOnly: true})
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2 (%v)", len(roots), roots)
	}

	teams, err := s.List(ctx, corecontainer.Filter{Kind: corecontainer.KindTeam})
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 1 || teams[0].ID != eng.ID {
		t.Fatalf("teams = %v, want just %s", teams, eng.ID)
	}

	// Name resolution — how a CLI turns "acme/eng" into a label without ever
	// putting the name in a policy.
	found, err := s.List(ctx, corecontainer.Filter{
		Kind: corecontainer.KindTeam, ParentID: acme.ID, Name: "eng",
	})
	if err != nil {
		t.Fatalf("resolve name: %v", err)
	}
	if len(found) != 1 || found[0].ID != eng.ID {
		t.Fatalf("resolve = %v, want just %s", found, eng.ID)
	}

	children, err := s.List(ctx, corecontainer.Filter{ParentID: globex.ID})
	if err != nil {
		t.Fatalf("list children: %v", err)
	}
	if len(children) != 0 {
		t.Fatalf("children of an empty org = %v", children)
	}
}

func TestReindexSubtreeLeavesUnrelatedResourcesAlone(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	acme := mustCreate(t, s, corecontainer.KindOrg, "acme", "")
	eng := mustCreate(t, s, corecontainer.KindTeam, "eng", acme.ID)
	globex := mustCreate(t, s, corecontainer.KindOrg, "globex", "")
	ops := mustCreate(t, s, corecontainer.KindTeam, "ops", globex.ID)

	if err := s.SetResourceContainers(ctx, "twin", "acme-twin", []string{eng.ID}); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if err := s.SetResourceContainers(ctx, "twin", "globex-twin", []string{ops.ID}); err != nil {
		t.Fatalf("scope: %v", err)
	}
	before, err := s.ScopesOf(ctx, "twin", "globex-twin")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}

	if err := s.ReindexSubtree(ctx, acme.ID); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	after, err := s.ScopesOf(ctx, "twin", "globex-twin")
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if !slices.Equal(before, after) {
		t.Fatalf("reindexing acme changed a globex twin: %v → %v", before, after)
	}
	if err := s.ReindexSubtree(ctx, "nope"); !errors.Is(err, coreerrors.ErrContainerNotFound) {
		t.Fatalf("reindex unknown err = %v, want ErrContainerNotFound", err)
	}
}
