package auth_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/auth"
)

// countingStore records writes so a test can assert that a login which changed
// nothing also wrote nothing, and injects failures for the error paths.
type countingStore struct {
	auth.Store

	putPrincipals int
	putBindings   int
	delBindings   int

	getPrincipalErr error
	putPrincipalErr error
	listBindingErr  error
	getRoleErr      error
	putBindingErr   error
	delBindingErr   error
}

func (s *countingStore) GetPrincipal(ctx context.Context, id string) (auth.Principal, error) {
	if s.getPrincipalErr != nil {
		return auth.Principal{}, s.getPrincipalErr
	}
	return s.Store.GetPrincipal(ctx, id)
}

func (s *countingStore) PutPrincipal(ctx context.Context, p auth.Principal) error {
	if s.putPrincipalErr != nil {
		return s.putPrincipalErr
	}
	s.putPrincipals++
	return s.Store.PutPrincipal(ctx, p)
}

func (s *countingStore) ListBindings(ctx context.Context, id string) ([]auth.RoleBinding, error) {
	if s.listBindingErr != nil {
		return nil, s.listBindingErr
	}
	return s.Store.ListBindings(ctx, id)
}

func (s *countingStore) GetRole(ctx context.Context, name string) (auth.Role, error) {
	if s.getRoleErr != nil {
		return auth.Role{}, s.getRoleErr
	}
	return s.Store.GetRole(ctx, name)
}

func (s *countingStore) PutBinding(ctx context.Context, b auth.RoleBinding) error {
	if s.putBindingErr != nil {
		return s.putBindingErr
	}
	s.putBindings++
	return s.Store.PutBinding(ctx, b)
}

func (s *countingStore) DeleteBinding(ctx context.Context, id string) error {
	if s.delBindingErr != nil {
		return s.delBindingErr
	}
	s.delBindings++
	return s.Store.DeleteBinding(ctx, id)
}

func newProvisionerStore(t *testing.T) *countingStore {
	t.Helper()
	base := auth.NewMemoryStore()
	for _, name := range []string{"admin", "operator", "viewer", "auditor"} {
		if err := base.PutRole(context.Background(), auth.Role{Name: name}); err != nil {
			t.Fatalf("seed role %q: %v", name, err)
		}
	}
	return &countingStore{Store: base}
}

func newProvisioner(t *testing.T, store auth.Store, m auth.RoleMap) *auth.Provisioner {
	t.Helper()
	return &auth.Provisioner{Store: store, Roles: m, Prefix: "oidc"}
}

func defaultMap() auth.RoleMap {
	return auth.RoleMap{Groups: map[string][]auth.RoleGrant{
		"karakuri-admins":    {{Role: "admin"}},
		"karakuri-operators": {{Role: "operator"}},
	}}
}

func roleNames(t *testing.T, store auth.Store, principalID string) []string {
	t.Helper()
	bindings, err := store.ListBindings(context.Background(), principalID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	var out []string
	for _, b := range bindings {
		out = append(out, b.Role)
	}
	slices.Sort(out)
	return out
}

func bindingScopes(t *testing.T, store auth.Store, principalID string) []string {
	t.Helper()
	bindings, err := store.ListBindings(context.Background(), principalID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	var out []string
	for _, b := range bindings {
		out = append(out, b.EffectiveScope())
	}
	slices.Sort(out)
	return out
}

func TestProvisionCreatesNamespacedPrincipal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())

	principal, change, err := p.Provision(ctx, auth.ExternalIdentity{
		Issuer:  "https://idp.example.com",
		Subject: "8f3c",
		Email:   "alice@example.com",
		Name:    "Alice",
		Groups:  []string{"karakuri-operators"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if principal.ID != "oidc:8f3c" {
		t.Errorf("principal ID = %q, want the namespaced form", principal.ID)
	}
	if principal.Name != "Alice" {
		t.Errorf("Name = %q, want the provider's display name", principal.Name)
	}
	if principal.Kind != auth.KindUser {
		t.Errorf("Kind = %q, want user", principal.Kind)
	}
	if principal.Attrs["email"] != "alice@example.com" || principal.Attrs["issuer"] != "https://idp.example.com" {
		t.Errorf("Attrs = %v, want email and issuer carried through", principal.Attrs)
	}
	if !change.Created {
		t.Error("change.Created = false on a first login")
	}
	if !slices.Equal(change.Added, []string{"operator"}) {
		t.Errorf("change.Added = %v, want [operator]", change.Added)
	}
	if got := roleNames(t, store, "oidc:8f3c"); !slices.Equal(got, []string{"operator"}) {
		t.Errorf("bound roles = %v, want [operator]", got)
	}
}

// A provider that asserts the local administrator's name must not reach the
// local administrator.
func TestProvisionCannotImpersonateLocalPrincipal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	if err := store.PutPrincipal(ctx, auth.Principal{ID: "admin", Name: "Bootstrap administrator"}); err != nil {
		t.Fatalf("seed local admin: %v", err)
	}
	if err := store.PutBinding(ctx, auth.RoleBinding{ID: "bootstrap-admin", PrincipalID: "admin", Role: "admin", Scope: "*"}); err != nil {
		t.Fatalf("seed local admin binding: %v", err)
	}

	p := newProvisioner(t, store, defaultMap())
	principal, _, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "admin", Groups: []string{"marketing"}})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if principal.ID != "oidc:admin" {
		t.Fatalf("principal ID = %q, want oidc:admin", principal.ID)
	}

	local, err := store.GetPrincipal(ctx, "admin")
	if err != nil {
		t.Fatalf("local admin disappeared: %v", err)
	}
	if local.Name != "Bootstrap administrator" {
		t.Errorf("local admin was overwritten: name = %q", local.Name)
	}
	if got := roleNames(t, store, "admin"); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("local admin roles = %v, want [admin] untouched", got)
	}
	if got := roleNames(t, store, "oidc:admin"); got != nil {
		t.Errorf("federated impostor holds %v, want nothing", got)
	}
}

func TestProvisionReconcilesRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-admins"}}

	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	if got := roleNames(t, store, "oidc:alice"); !slices.Equal(got, []string{"admin"}) {
		t.Fatalf("after first login = %v, want [admin]", got)
	}

	// Moved from admins to operators at the provider.
	identity.Groups = []string{"karakuri-operators"}
	_, change, err := p.Provision(ctx, identity)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if !slices.Equal(change.Added, []string{"operator"}) || !slices.Equal(change.Removed, []string{"admin"}) {
		t.Errorf("change = %+v, want operator added and admin removed", change)
	}
	if got := roleNames(t, store, "oidc:alice"); !slices.Equal(got, []string{"operator"}) {
		t.Fatalf("after the move = %v, want [operator]", got)
	}

	// Removed from every mapped group: the grant goes away.
	identity.Groups = nil
	if _, change, err = p.Provision(ctx, identity); err != nil {
		t.Fatalf("third login: %v", err)
	}
	if !slices.Equal(change.Removed, []string{"operator"}) {
		t.Errorf("change.Removed = %v, want [operator]", change.Removed)
	}
	if got := roleNames(t, store, "oidc:alice"); got != nil {
		t.Fatalf("after leaving every group = %v, want nothing", got)
	}
}

// The hole Phase 16 opened: every federated user landed with their mapped role
// over everything, so a directory group of two hundred people was two hundred
// globally-scoped principals.
func TestProvisionHonoursTheScopeOnAGrant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, auth.RoleMap{Groups: map[string][]auth.RoleGrant{
		"acme-engineers": {{Role: "operator", Scope: "team:t_7f2a"}},
	}})

	_, change, err := p.Provision(ctx, auth.ExternalIdentity{
		Subject: "alice", Groups: []string{"acme-engineers"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if !slices.Equal(change.Added, []string{"operator@team:t_7f2a"}) {
		t.Errorf("change.Added = %v, want the scope named", change.Added)
	}

	bindings, err := store.ListBindings(ctx, "oidc:alice")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings = %+v, want one", bindings)
	}
	if bindings[0].Scope != "team:t_7f2a" {
		t.Fatalf("scope = %q, want the team — a federated login must not grant everything", bindings[0].Scope)
	}
	// And it means what it says: the binding reaches a twin inside that team
	// and nothing outside it.
	inTeam := auth.Resource("twin", "abc").WithScopes("team:t_7f2a", "org:o_9c31")
	elsewhere := auth.Resource("twin", "xyz").WithScopes("team:t_be04", "org:o_1111")
	if !inTeam.InScope(bindings[0].Scope) {
		t.Error("the binding does not reach a twin in its own team")
	}
	if elsewhere.InScope(bindings[0].Scope) {
		t.Error("the binding reaches another tenant's twin")
	}
}

// Somebody in two teams holds the role in both, and losing one group at the
// provider has to take one binding away and leave the other.
func TestProvisionReconcilesPerScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, auth.RoleMap{Groups: map[string][]auth.RoleGrant{
		"acme-engineers":   {{Role: "operator", Scope: "team:t_7f2a"}},
		"globex-engineers": {{Role: "operator", Scope: "team:t_be04"}},
	}})
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"acme-engineers", "globex-engineers"}}

	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	if got := bindingScopes(t, store, "oidc:alice"); !slices.Equal(got, []string{"team:t_7f2a", "team:t_be04"}) {
		t.Fatalf("scopes = %v, want one binding per team", got)
	}

	identity.Groups = []string{"acme-engineers"}
	_, change, err := p.Provision(ctx, identity)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if !slices.Equal(change.Removed, []string{"operator@team:t_be04"}) {
		t.Errorf("change.Removed = %v, want only the globex grant", change.Removed)
	}
	if got := bindingScopes(t, store, "oidc:alice"); !slices.Equal(got, []string{"team:t_7f2a"}) {
		t.Fatalf("scopes = %v, want the remaining team", got)
	}
}

// A binding written before scopes existed carries Scope "*" and an ID with no
// scope in it. Reconciliation keys on the binding's own fields, so an unscoped
// map recognises it as the grant it already is rather than deleting it and
// writing an identical one under a new ID.
func TestProvisionLeavesPreScopeBindingsInPlace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-operators"}}

	legacy := auth.RoleBinding{
		ID: "idp:oidc:alice:operator", PrincipalID: "oidc:alice", Role: "operator", Scope: "*",
	}
	if err := store.PutBinding(ctx, legacy); err != nil {
		t.Fatalf("legacy binding: %v", err)
	}

	_, change, err := p.Provision(ctx, identity)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	// The principal itself is new, so Created is set; what matters is that no
	// binding was added or removed — the grant is already in place.
	if len(change.Added) != 0 || len(change.Removed) != 0 {
		t.Errorf("change = %+v, want no binding churn", change)
	}
	bindings, err := store.ListBindings(ctx, "oidc:alice")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != legacy.ID {
		t.Fatalf("bindings = %+v, want the original untouched", bindings)
	}
}

// A grant an administrator made by hand is not the provider's to revoke.
func TestProvisionLeavesUnmanagedBindingsAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-operators"}}

	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	// The shape the API mints: <principal>:<role>:<scope>, no idp: prefix.
	if err := store.PutBinding(ctx, auth.RoleBinding{
		ID: "oidc:alice:auditor:*", PrincipalID: "oidc:alice", Role: "auditor", Scope: "*",
	}); err != nil {
		t.Fatalf("hand-made binding: %v", err)
	}

	identity.Groups = nil
	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("second login: %v", err)
	}
	if got := roleNames(t, store, "oidc:alice"); !slices.Equal(got, []string{"auditor"}) {
		t.Fatalf("roles = %v, want the hand-made auditor grant to survive", got)
	}
}

// Almost every login changes nothing, and paying a write for each would make
// the store the bottleneck on a busy morning.
func TestProvisionWritesNothingWhenNothingChanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())
	identity := auth.ExternalIdentity{Subject: "alice", Name: "Alice", Groups: []string{"karakuri-operators"}}

	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	principals, bindings := store.putPrincipals, store.putBindings

	_, change, err := p.Provision(ctx, identity)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if !change.Empty() {
		t.Errorf("change = %+v, want empty on an unchanged login", change)
	}
	if store.putPrincipals != principals || store.putBindings != bindings || store.delBindings != 0 {
		t.Errorf("second login wrote: principals +%d, bindings +%d, deletes %d",
			store.putPrincipals-principals, store.putBindings-bindings, store.delBindings)
	}
}

func TestProvisionUpdatesChangedProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())

	if _, _, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice", Name: "Alice"}); err != nil {
		t.Fatalf("first login: %v", err)
	}
	principal, _, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice", Name: "Alice Cooper", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if principal.Name != "Alice Cooper" || principal.Attrs["email"] != "alice@example.com" {
		t.Errorf("principal = %+v, want the renamed profile", principal)
	}
	if store.putPrincipals != 2 {
		t.Errorf("PutPrincipal calls = %d, want 2", store.putPrincipals)
	}
}

// Disabling an account has to outrank the provider still authenticating it.
func TestProvisionRefusesDisabledPrincipal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-admins"}}

	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	if err := store.PutPrincipal(ctx, auth.Principal{ID: "oidc:alice", Kind: auth.KindUser, Disabled: true}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if _, _, err := p.Provision(ctx, identity); !errors.Is(err, auth.ErrPrincipalDisabled) {
		t.Fatalf("err = %v, want ErrPrincipalDisabled", err)
	}
}

// A stale mapping should not lock the whole company out; Validate is where a
// typo is meant to be caught.
func TestProvisionSkipsUnknownRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, auth.RoleMap{Groups: map[string][]auth.RoleGrant{
		"eng": {{Role: "operator"}, {Role: "wizard"}},
	}})

	_, change, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice", Groups: []string{"eng"}})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if !slices.Equal(change.Unknown, []string{"wizard"}) {
		t.Errorf("change.Unknown = %v, want [wizard]", change.Unknown)
	}
	if got := roleNames(t, store, "oidc:alice"); !slices.Equal(got, []string{"operator"}) {
		t.Errorf("roles = %v, want the known role still granted", got)
	}
}

func TestProvisionOnChangeHook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)

	var calls []auth.RoleChange
	p := newProvisioner(t, store, defaultMap())
	p.OnChange = func(_ context.Context, _ auth.Principal, c auth.RoleChange) { calls = append(calls, c) }

	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-operators"}}
	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("first login: %v", err)
	}
	if _, _, err := p.Provision(ctx, identity); err != nil {
		t.Fatalf("second login: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("OnChange calls = %d, want 1 — the unchanged login must not fire it", len(calls))
	}
	if !calls[0].Created || !slices.Equal(calls[0].Added, []string{"operator"}) {
		t.Errorf("change = %+v", calls[0])
	}
}

func TestProvisionValidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)

	ok := newProvisioner(t, store, defaultMap())
	if err := ok.Validate(ctx); err != nil {
		t.Fatalf("Validate on a good map: %v", err)
	}

	typo := newProvisioner(t, store, auth.RoleMap{Groups: map[string][]auth.RoleGrant{"eng": {{Role: "opperator"}}}})
	if err := typo.Validate(ctx); !errors.Is(err, auth.ErrRoleNotFound) {
		t.Fatalf("Validate on a typo = %v, want ErrRoleNotFound", err)
	}

	noPrefix := &auth.Provisioner{Store: store, Roles: defaultMap()}
	if err := noPrefix.Validate(ctx); !errors.Is(err, auth.ErrNoPrefix) {
		t.Fatalf("Validate without a prefix = %v, want ErrNoPrefix", err)
	}

	noStore := &auth.Provisioner{Prefix: "oidc"}
	if err := noStore.Validate(ctx); err == nil {
		t.Fatal("Validate without a store returned nil")
	}
}

func TestProvisionErrorPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("boom")
	identity := auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-operators"}}

	cases := []struct {
		name    string
		mutate  func(*countingStore)
		wantErr error
	}{
		{name: "get principal", mutate: func(s *countingStore) { s.getPrincipalErr = boom }, wantErr: boom},
		{name: "put principal", mutate: func(s *countingStore) { s.putPrincipalErr = boom }, wantErr: boom},
		{name: "list bindings", mutate: func(s *countingStore) { s.listBindingErr = boom }, wantErr: boom},
		{name: "get role", mutate: func(s *countingStore) { s.getRoleErr = boom }, wantErr: boom},
		{name: "put binding", mutate: func(s *countingStore) { s.putBindingErr = boom }, wantErr: boom},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newProvisionerStore(t)
			tc.mutate(store)
			p := newProvisioner(t, store, defaultMap())
			if _, _, err := p.Provision(ctx, identity); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("delete binding", func(t *testing.T) {
		t.Parallel()
		store := newProvisionerStore(t)
		p := newProvisioner(t, store, defaultMap())
		if _, _, err := p.Provision(ctx, identity); err != nil {
			t.Fatalf("seed login: %v", err)
		}
		store.delBindingErr = boom
		if _, _, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice"}); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
	})

	t.Run("no store", func(t *testing.T) {
		t.Parallel()
		p := &auth.Provisioner{Prefix: "oidc"}
		if _, _, err := p.Provision(ctx, identity); err == nil {
			t.Fatal("Provision without a store returned nil")
		}
	})

	t.Run("bad identity", func(t *testing.T) {
		t.Parallel()
		p := newProvisioner(t, newProvisionerStore(t), defaultMap())
		if _, _, err := p.Provision(ctx, auth.ExternalIdentity{}); !errors.Is(err, auth.ErrNoSubject) {
			t.Fatalf("err = %v, want ErrNoSubject", err)
		}
	})
}

// A binding whose deletion races another writer is already in the desired
// state, so a missing row is not an error.
func TestProvisionToleratesAlreadyDeletedBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newProvisionerStore(t)
	p := newProvisioner(t, store, defaultMap())

	if _, _, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice", Groups: []string{"karakuri-operators"}}); err != nil {
		t.Fatalf("seed login: %v", err)
	}
	store.delBindingErr = auth.ErrBindingNotFound
	if _, change, err := p.Provision(ctx, auth.ExternalIdentity{Subject: "alice"}); err != nil {
		t.Fatalf("Provision: %v", err)
	} else if !slices.Equal(change.Removed, []string{"operator"}) {
		t.Errorf("change.Removed = %v, want [operator]", change.Removed)
	}
}
