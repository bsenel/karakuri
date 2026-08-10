package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// fixture builds a store with the role shape a real deployment uses: viewer is
// read-only, operator adds writes but is explicitly denied auth:write, admin
// inherits operator and holds the wildcard.
func fixture(t *testing.T) (*MemoryStore, *StoreAuthorizer) {
	t.Helper()
	ctx := context.Background()
	s := NewMemoryStore()
	roles := []Role{
		{Name: "viewer", System: true, Policies: []Policy{
			Allow("viewer-read", "twin:read", "*"),
			Allow("viewer-loop-read", "loop:read", "*"),
		}},
		{Name: "operator", System: true, Inherits: []string{"viewer"}, Policies: []Policy{
			Allow("operator-twin", "twin:*", "*"),
			Allow("operator-loop", "loop:*", "*"),
			// Explicit deny: an operator must never edit the auth model, even
			// though admin's wildcard would otherwise reach it via inheritance.
			Deny("operator-no-auth-write", "auth:write", "*"),
		}},
		{Name: "admin", System: true, Inherits: []string{"operator"}, Policies: []Policy{
			Allow("admin-all", "*", "*"),
		}},
		{Name: "owner-only", Policies: []Policy{
			Allow("own-update", "twin:update", "twin:*").When(Condition{Kind: CondOwnerEquals}),
		}},
	}
	for _, r := range roles {
		mustPut(t, s.PutRole(ctx, r))
	}
	return s, NewAuthorizer(s)
}

func bind(t *testing.T, s *MemoryStore, id, principal, role, scope string) {
	t.Helper()
	mustPut(t, s.PutBinding(context.Background(), RoleBinding{ID: id, PrincipalID: principal, Role: role, Scope: scope}))
}

func TestAuthorizeBasicRoles(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	bind(t, s, "b-vera", "vera", "viewer", "*")
	bind(t, s, "b-olive", "olive", "operator", "*")

	vera := Principal{ID: "vera"}
	olive := Principal{ID: "olive"}

	cases := []struct {
		name     string
		p        Principal
		action   Action
		res      ResourceRef
		want     bool
		wantRole string
	}{
		{"viewer reads", vera, "twin:read", Collection("twin"), true, "viewer"},
		{"viewer cannot write", vera, "twin:update", Resource("twin", "t1"), false, ""},
		{"viewer cannot start loops", vera, "loop:start", Collection("loop"), false, ""},
		{"operator writes", olive, "twin:update", Resource("twin", "t1"), true, "operator"},
		{"operator starts loops", olive, "loop:start", Collection("loop"), true, "operator"},
		{"operator inherits viewer", olive, "twin:read", Collection("twin"), true, "operator"},
		{"operator cannot read audit", olive, "audit:read", Collection("audit"), false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := a.Authorize(ctx, c.p, c.action, c.res)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if d.Allowed != c.want {
				t.Fatalf("Allowed = %v, want %v (%s)", d.Allowed, c.want, d.Reason)
			}
			if c.want && d.ViaRole != c.wantRole {
				t.Errorf("ViaRole = %q, want %q", d.ViaRole, c.wantRole)
			}
			if d.Resource != c.res.String() || d.Action != c.action {
				t.Errorf("decision echoed %s/%s", d.Action, d.Resource)
			}
		})
	}
}

func TestAuthorizeDenyWins(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	// admin inherits operator, which explicitly denies auth:write. Deny wins
	// over admin's own "*" allow no matter how specific the allow is.
	bind(t, s, "b-root", "root", "admin", "*")
	root := Principal{ID: "root"}

	d, err := a.Authorize(ctx, root, "auth:write", Collection("auth"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if d.Allowed {
		t.Fatal("deny did not win over an inherited wildcard allow")
	}
	if d.MatchedPolicy != "operator-no-auth-write" || d.ViaRole != "operator" {
		t.Errorf("trace = policy %q via %q", d.MatchedPolicy, d.ViaRole)
	}
	if !strings.Contains(d.Reason, "explicit deny") {
		t.Errorf("reason = %q", d.Reason)
	}

	// Everything else admin does is still allowed.
	if d, _ := a.Authorize(ctx, root, "audit:read", Collection("audit")); !d.Allowed {
		t.Errorf("admin denied audit:read: %s", d.Reason)
	}
}

func TestAuthorizeDenyWinsRegardlessOfBindingOrder(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	mustPut(t, s.PutRole(ctx, Role{Name: "granter", Policies: []Policy{Allow("g", "twin:update", "*")}}))
	mustPut(t, s.PutRole(ctx, Role{Name: "blocker", Policies: []Policy{Deny("d", "twin:update", "twin:t1")}}))
	a := NewAuthorizer(s)

	// The allow is evaluated first (binding IDs sort b1 < b2) but the later
	// deny still wins — scanning does not stop at the first allow.
	bind(t, s, "b1", "alice", "granter", "*")
	bind(t, s, "b2", "alice", "blocker", "*")

	d, err := a.Authorize(ctx, Principal{ID: "alice"}, "twin:update", Resource("twin", "t1"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if d.Allowed {
		t.Fatal("a deny in a later binding did not win")
	}
	if d.MatchedPolicy != "d" {
		t.Errorf("MatchedPolicy = %q, want d", d.MatchedPolicy)
	}
	// A different twin is unaffected by the narrowly-scoped deny.
	if d, _ := a.Authorize(ctx, Principal{ID: "alice"}, "twin:update", Resource("twin", "t2")); !d.Allowed {
		t.Errorf("unrelated resource denied: %s", d.Reason)
	}
}

func TestAuthorizeScopedBindings(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	// Olive operates one twin only.
	bind(t, s, "b-olive", "olive", "operator", "twin:abc")
	olive := Principal{ID: "olive"}

	if d, _ := a.Authorize(ctx, olive, "twin:update", Resource("twin", "abc")); !d.Allowed {
		t.Errorf("in-scope update denied: %s", d.Reason)
	} else if d.BindingScope != "twin:abc" {
		t.Errorf("BindingScope = %q", d.BindingScope)
	}

	d, _ := a.Authorize(ctx, olive, "twin:update", Resource("twin", "xyz"))
	if d.Allowed {
		t.Fatal("out-of-scope update allowed")
	}
	if len(d.ConsideredRoles) != 0 || !strings.Contains(d.Reason, "covers") {
		t.Errorf("expected a 'no binding covers' reason, got %q (roles %v)", d.Reason, d.ConsideredRoles)
	}

	// The collection is outside a single-object scope too.
	if d, _ := a.Authorize(ctx, olive, "twin:read", Collection("twin")); d.Allowed {
		t.Error("single-twin scope granted access to the whole collection")
	}
}

func TestAuthorizeConditions(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	bind(t, s, "b-alice", "alice", "owner-only", "*")
	alice := Principal{ID: "alice"}

	d, err := a.Authorize(ctx, alice, "twin:update", Resource("twin", "t1").WithOwner("alice"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !d.Allowed {
		t.Fatalf("owner update denied: %s", d.Reason)
	}
	if len(d.Conditions) != 1 || !d.Conditions[0].Satisfied {
		t.Errorf("condition trace = %+v", d.Conditions)
	}

	d, _ = a.Authorize(ctx, alice, "twin:update", Resource("twin", "t1").WithOwner("bob"))
	if d.Allowed {
		t.Fatal("non-owner update allowed")
	}
	// The role was in play, so the reason is a default deny rather than a
	// scope miss — the distinction matters when debugging a policy.
	if !slices.Contains(d.ConsideredRoles, "owner-only") || !strings.Contains(d.Reason, "default deny") {
		t.Errorf("reason = %q, roles = %v", d.Reason, d.ConsideredRoles)
	}

	if d, _ := a.Authorize(ctx, alice, "twin:update", Resource("twin", "t1")); d.Allowed {
		t.Error("unowned resource satisfied owner_equals")
	}
}

func TestAuthorizeConditionalDeny(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	mustPut(t, s.PutRole(ctx, Role{Name: "prod-guard", Policies: []Policy{
		Allow("allow-all-twins", "twin:*", "*"),
		Deny("no-prod", "twin:update", "*").When(Condition{Kind: CondAttrEquals, Key: "resource.env", Values: []string{"prod"}}),
	}}))
	a := NewAuthorizer(s)
	bind(t, s, "b1", "alice", "prod-guard", "*")
	alice := Principal{ID: "alice"}

	d, _ := a.Authorize(ctx, alice, "twin:update", Resource("twin", "t1").WithAttr("env", "prod"))
	if d.Allowed {
		t.Fatal("conditional deny did not fire on prod")
	}
	d, _ = a.Authorize(ctx, alice, "twin:update", Resource("twin", "t1").WithAttr("env", "dev"))
	if !d.Allowed {
		t.Fatalf("conditional deny fired on dev: %s", d.Reason)
	}
}

func TestAuthorizeRejections(t *testing.T) {
	ctx := context.Background()
	s, a := fixture(t)
	bind(t, s, "b-vera", "vera", "viewer", "*")

	cases := []struct {
		name       string
		p          Principal
		wantReason string
	}{
		{"anonymous", Principal{}, "no principal"},
		{"disabled", Principal{ID: "vera", Disabled: true}, "disabled"},
		{"unbound", Principal{ID: "nobody"}, "no role bindings"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := a.Authorize(ctx, c.p, "twin:read", Collection("twin"))
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if d.Allowed {
				t.Fatal("expected denial")
			}
			if !strings.Contains(d.Reason, c.wantReason) {
				t.Errorf("Reason = %q, want it to mention %q", d.Reason, c.wantReason)
			}
		})
	}
}

// failingStore injects store errors so the authorizer's error paths are covered.
type failingStore struct {
	Store
	bindingErr error
	roleErr    error
}

func (f failingStore) ListBindings(ctx context.Context, id string) ([]RoleBinding, error) {
	if f.bindingErr != nil {
		return nil, f.bindingErr
	}
	return f.Store.ListBindings(ctx, id)
}

func (f failingStore) GetRole(ctx context.Context, name string) (Role, error) {
	if f.roleErr != nil {
		return Role{}, f.roleErr
	}
	return f.Store.GetRole(ctx, name)
}

func TestAuthorizeStoreErrors(t *testing.T) {
	ctx := context.Background()
	base, _ := fixture(t)
	bind(t, base, "b-vera", "vera", "viewer", "*")
	vera := Principal{ID: "vera"}
	boom := errors.New("boom")

	a := NewAuthorizer(failingStore{Store: base, bindingErr: boom})
	if _, err := a.Authorize(ctx, vera, "twin:read", Collection("twin")); !errors.Is(err, boom) {
		t.Errorf("binding error = %v, want boom", err)
	}

	a = NewAuthorizer(failingStore{Store: base, roleErr: boom})
	if _, err := a.Authorize(ctx, vera, "twin:read", Collection("twin")); !errors.Is(err, boom) {
		t.Errorf("role error = %v, want boom", err)
	}

	// A binding naming a role nobody registered is a configuration error, not
	// a silent denial.
	bind(t, base, "b-ghost", "ghost", "does-not-exist", "*")
	a = NewAuthorizer(base)
	if _, err := a.Authorize(ctx, Principal{ID: "ghost"}, "twin:read", Collection("twin")); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("missing role = %v, want ErrRoleNotFound", err)
	}

	// Same for a role that exists but inherits one that does not.
	mustPut(t, base.PutRole(ctx, Role{Name: "half-orphan", Inherits: []string{"nowhere"}}))
	bind(t, base, "b-half", "half", "half-orphan", "*")
	if _, err := a.Authorize(ctx, Principal{ID: "half"}, "twin:read", Collection("twin")); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("missing inherited role = %v, want ErrRoleNotFound", err)
	}
}

func TestAuthorizeRoleCycleSurfaces(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	mustPut(t, s.PutRole(ctx, Role{Name: "a", Inherits: []string{"b"}}))
	mustPut(t, s.PutRole(ctx, Role{Name: "b", Inherits: []string{"a"}}))
	bind(t, s, "b1", "alice", "a", "*")

	if _, err := NewAuthorizer(s).Authorize(ctx, Principal{ID: "alice"}, "twin:read", Collection("twin")); !errors.Is(err, ErrRoleCycle) {
		t.Errorf("cycle = %v, want ErrRoleCycle", err)
	}
}
