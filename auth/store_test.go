package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestMemoryStorePrincipals(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.PutPrincipal(ctx, Principal{}); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("PutPrincipal without ID = %v", err)
	}
	if _, err := s.GetPrincipal(ctx, "alice"); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("GetPrincipal(missing) = %v", err)
	}
	if err := s.DeletePrincipal(ctx, "alice"); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("DeletePrincipal(missing) = %v", err)
	}

	alice := Principal{ID: "alice", Name: "Alice", Attrs: map[string]string{"team": "eng"}}
	mustPut(t, s.PutPrincipal(ctx, alice))
	mustPut(t, s.PutPrincipal(ctx, Principal{ID: "bob"}))

	got, err := s.GetPrincipal(ctx, "alice")
	if err != nil {
		t.Fatalf("GetPrincipal: %v", err)
	}
	got.Attrs["team"] = "hr"
	again, _ := s.GetPrincipal(ctx, "alice")
	if again.Attrs["team"] != "eng" {
		t.Fatal("GetPrincipal handed out an aliased Attrs map")
	}

	list, err := s.ListPrincipals(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "alice" || list[1].ID != "bob" {
		t.Fatalf("ListPrincipals = %+v, %v", list, err)
	}

	// Deleting a principal must not leave its grants behind.
	mustPut(t, s.PutBinding(ctx, RoleBinding{ID: "b1", PrincipalID: "alice", Role: "viewer"}))
	mustPut(t, s.PutBinding(ctx, RoleBinding{ID: "b2", PrincipalID: "bob", Role: "viewer"}))
	mustPut(t, s.DeletePrincipal(ctx, "alice"))
	bindings, _ := s.ListBindings(ctx, "")
	if len(bindings) != 1 || bindings[0].ID != "b2" {
		t.Fatalf("bindings after principal delete = %+v", bindings)
	}
}

func TestMemoryStoreRoles(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.PutRole(ctx, Role{}); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("PutRole without a name = %v", err)
	}
	if _, err := s.GetRole(ctx, "viewer"); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("GetRole(missing) = %v", err)
	}
	if err := s.DeleteRole(ctx, "viewer"); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("DeleteRole(missing) = %v", err)
	}

	viewer := Role{Name: "viewer", System: true, Policies: []Policy{Allow("v1", "twin:read", "*")}}
	mustPut(t, s.PutRole(ctx, viewer))
	mustPut(t, s.PutRole(ctx, Role{Name: "custom", Policies: []Policy{Allow("c1", "twin:update", "*")}}))

	got, err := s.GetRole(ctx, "viewer")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	got.Policies[0].Action = "twin:delete"
	again, _ := s.GetRole(ctx, "viewer")
	if again.Policies[0].Action != "twin:read" {
		t.Fatal("GetRole handed out aliased policies")
	}

	// System roles are immutable — an operator editing "admin" is how everyone
	// gets locked out.
	if err := s.PutRole(ctx, Role{Name: "viewer"}); !errors.Is(err, ErrSystemRole) {
		t.Errorf("overwriting a system role = %v, want ErrSystemRole", err)
	}
	if err := s.DeleteRole(ctx, "viewer"); !errors.Is(err, ErrSystemRole) {
		t.Errorf("deleting a system role = %v, want ErrSystemRole", err)
	}
	// Re-seeding the same system role (System still true) is allowed, so
	// bootstrap stays idempotent.
	mustPut(t, s.PutRole(ctx, viewer))

	roles, err := s.ListRoles(ctx)
	if err != nil || len(roles) != 2 || roles[0].Name != "custom" || roles[1].Name != "viewer" {
		t.Fatalf("ListRoles = %+v, %v", roles, err)
	}
	mustPut(t, s.DeleteRole(ctx, "custom"))
}

func TestMemoryStoreBindings(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	bad := []RoleBinding{
		{PrincipalID: "alice", Role: "viewer"},
		{ID: "b", Role: "viewer"},
		{ID: "b", PrincipalID: "alice"},
	}
	for _, b := range bad {
		if err := s.PutBinding(ctx, b); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("PutBinding(%+v) = %v, want ErrInvalidPattern", b, err)
		}
	}
	if err := s.PutBinding(ctx, RoleBinding{ID: "b", PrincipalID: "a", Role: "viewer", Scope: "tw*in"}); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("PutBinding with a bad scope = %v", err)
	}
	if err := s.DeleteBinding(ctx, "nope"); !errors.Is(err, ErrBindingNotFound) {
		t.Errorf("DeleteBinding(missing) = %v", err)
	}

	mustPut(t, s.PutBinding(ctx, RoleBinding{ID: "b1", PrincipalID: "alice", Role: "viewer"}))
	mustPut(t, s.PutBinding(ctx, RoleBinding{ID: "b2", PrincipalID: "alice", Role: "operator", Scope: "twin:abc"}))
	mustPut(t, s.PutBinding(ctx, RoleBinding{ID: "b3", PrincipalID: "bob", Role: "viewer"}))

	mine, err := s.ListBindings(ctx, "alice")
	if err != nil || len(mine) != 2 {
		t.Fatalf("ListBindings(alice) = %+v, %v", mine, err)
	}
	all, _ := s.ListBindings(ctx, "")
	if len(all) != 3 {
		t.Fatalf("ListBindings(all) = %+v", all)
	}
	mustPut(t, s.DeleteBinding(ctx, "b1"))
	mine, _ = s.ListBindings(ctx, "alice")
	if len(mine) != 1 || mine[0].ID != "b2" {
		t.Fatalf("after delete = %+v", mine)
	}
}

func TestMemoryStoreConcurrent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	mustPut(t, s.PutRole(ctx, Role{Name: "viewer", Policies: []Policy{Allow("v1", "twin:read", "*")}}))

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("p%02d", i)
			_ = s.PutPrincipal(ctx, Principal{ID: id})
			_ = s.PutBinding(ctx, RoleBinding{ID: "b" + id, PrincipalID: id, Role: "viewer"})
			_, _ = s.GetPrincipal(ctx, id)
			_, _ = s.ListPrincipals(ctx)
			_, _ = s.ListRoles(ctx)
			_, _ = s.ListBindings(ctx, id)
		}(i)
	}
	wg.Wait()

	list, _ := s.ListPrincipals(ctx)
	if len(list) != 32 {
		t.Fatalf("got %d principals, want 32", len(list))
	}
}

func mustPut(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("store write: %v", err)
	}
}
