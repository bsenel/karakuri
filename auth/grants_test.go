package auth

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestGrants(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	bind(t, s, "b1", "olive", "operator", "twin:*")
	bind(t, s, "b2", "olive", "viewer", "*")

	grants, err := a.Grants(ctx, "olive")
	if err != nil {
		t.Fatalf("Grants: %v", err)
	}
	// operator flattens to its own three policies plus viewer's two, and the
	// standalone viewer binding contributes those two again under a wider scope.
	if len(grants) != 7 {
		t.Fatalf("got %d grants, want 7: %+v", len(grants), grants)
	}
	if !slices.IsSortedFunc(grants, func(x, y Grant) int {
		return int(sign(string(x.Action), string(y.Action)))
	}) {
		t.Errorf("grants are not sorted by action: %+v", grants)
	}

	var scopes []string
	for _, g := range grants {
		if g.PolicyID == "viewer-read" {
			scopes = append(scopes, g.Scope)
		}
	}
	slices.Sort(scopes)
	if !slices.Equal(scopes, []string{"*", "twin:*"}) {
		t.Errorf("viewer-read reached through scopes %v", scopes)
	}

	if grants, err := a.Grants(ctx, "nobody"); err != nil || len(grants) != 0 {
		t.Errorf("Grants(unbound) = %+v, %v", grants, err)
	}
}

func sign(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func TestRoles(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	bind(t, s, "b1", "olive", "operator", "twin:*")
	bind(t, s, "b2", "olive", "operator", "loop:*")
	bind(t, s, "b3", "olive", "viewer", "*")

	roles, err := a.Roles(ctx, "olive")
	if err != nil {
		t.Fatalf("Roles: %v", err)
	}
	// Distinct and sorted — two operator bindings collapse to one role name.
	if !slices.Equal(roles, []string{"operator", "viewer"}) {
		t.Errorf("Roles = %v", roles)
	}
	if roles, _ := a.Roles(ctx, "nobody"); len(roles) != 0 {
		t.Errorf("Roles(unbound) = %v", roles)
	}
}

func TestExpandGrants(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	catalog := NewCatalog()
	for _, action := range []Action{"twin:read", "twin:update", "loop:read", "loop:start", "audit:read", "auth:read", "auth:write"} {
		catalog.MustRegister(action, string(action))
	}

	bind(t, s, "b-vera", "vera", "viewer", "*")
	got, err := a.ExpandGrants(ctx, "vera", catalog)
	if err != nil {
		t.Fatalf("ExpandGrants: %v", err)
	}
	if !slices.Equal(got, []Action{"loop:read", "twin:read"}) {
		t.Errorf("viewer permissions = %v", got)
	}

	// admin's wildcard expands to everything except the auth:write its
	// inherited operator role denies unconditionally.
	bind(t, s, "b-root", "root", "admin", "*")
	got, err = a.ExpandGrants(ctx, "root", catalog)
	if err != nil {
		t.Fatalf("ExpandGrants: %v", err)
	}
	want := []Action{"audit:read", "auth:read", "loop:read", "loop:start", "twin:read", "twin:update"}
	if !slices.Equal(got, want) {
		t.Errorf("admin permissions = %v, want %v", got, want)
	}
}

func TestExpandGrantsKeepsConditionalDenies(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	mustPut(t, s.PutRole(ctx, Role{Name: "r", Policies: []Policy{
		Allow("a", "twin:*", "*"),
		// Conditional: whether it bites depends on the resource, so it must not
		// be subtracted from the "what can I do" list.
		Deny("d", "twin:update", "*").When(Condition{Kind: CondOwnerEquals}),
	}}))
	bind(t, s, "b1", "alice", "r", "*")

	catalog := NewCatalog()
	catalog.MustRegister("twin:read", "read")
	catalog.MustRegister("twin:update", "update")

	got, err := NewAuthorizer(s).ExpandGrants(ctx, "alice", catalog)
	if err != nil {
		t.Fatalf("ExpandGrants: %v", err)
	}
	if !slices.Equal(got, []Action{"twin:read", "twin:update"}) {
		t.Errorf("permissions = %v, want both actions retained", got)
	}
}

func TestGrantsStoreErrors(t *testing.T) {
	ctx := context.Background()
	base, _ := fixture(t)
	bind(t, base, "b1", "alice", "viewer", "*")
	boom := errors.New("boom")

	a := NewAuthorizer(failingStore{Store: base, bindingErr: boom})
	if _, err := a.Grants(ctx, "alice"); !errors.Is(err, boom) {
		t.Errorf("Grants error = %v", err)
	}
	if _, err := a.Roles(ctx, "alice"); !errors.Is(err, boom) {
		t.Errorf("Roles error = %v", err)
	}
	if _, err := a.ExpandGrants(ctx, "alice", NewCatalog()); !errors.Is(err, boom) {
		t.Errorf("ExpandGrants error = %v", err)
	}

	a = NewAuthorizer(failingStore{Store: base, roleErr: boom})
	if _, err := a.Grants(ctx, "alice"); !errors.Is(err, boom) {
		t.Errorf("Grants role error = %v", err)
	}

	// A cycle surfaces from Grants too, not just from Authorize.
	cyclic := NewMemoryStore()
	mustPut(t, cyclic.PutRole(ctx, Role{Name: "a", Inherits: []string{"b"}}))
	mustPut(t, cyclic.PutRole(ctx, Role{Name: "b", Inherits: []string{"a"}}))
	bind(t, cyclic, "b1", "alice", "a", "*")
	if _, err := NewAuthorizer(cyclic).Grants(ctx, "alice"); !errors.Is(err, ErrRoleCycle) {
		t.Errorf("Grants cycle = %v, want ErrRoleCycle", err)
	}
}
