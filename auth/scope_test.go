package auth_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/auth"
)

// Containers used across these tests. Two organisations, and both of them have
// a team their people call "eng" — which is the case the whole ID-not-name rule
// exists for.
const (
	orgAcme       = "org:o_acme"
	orgGlobex     = "org:o_globex"
	teamAcmeEng   = "team:t_7f2a" // acme's "eng"
	teamGlobexEng = "team:t_be04" // globex's "eng"
	projectDelta  = "project:p_delta"
)

func scopeStore(t *testing.T) *auth.MemoryStore {
	t.Helper()
	ctx := context.Background()
	store := auth.NewMemoryStore()

	if err := store.PutRole(ctx, auth.Role{
		Name: "operator",
		Policies: []auth.Policy{
			{ID: "twin-rw", Action: "twin:*", Resource: "twin:*", Effect: auth.EffectAllow},
		},
	}); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	if err := store.PutRole(ctx, auth.Role{
		Name: "viewer",
		Policies: []auth.Policy{
			{ID: "twin-r", Action: "twin:read", Resource: "twin:*", Effect: auth.EffectAllow},
		},
	}); err != nil {
		t.Fatalf("seed viewer: %v", err)
	}
	if err := store.PutRole(ctx, auth.Role{
		Name: "quarantined",
		Policies: []auth.Policy{
			{ID: "twin-no", Action: "twin:*", Resource: "twin:*", Effect: auth.EffectDeny},
		},
	}); err != nil {
		t.Fatalf("seed quarantined: %v", err)
	}
	for _, p := range []string{"alice", "bob"} {
		if err := store.PutPrincipal(ctx, auth.Principal{ID: p}); err != nil {
			t.Fatalf("seed principal %q: %v", p, err)
		}
	}
	return store
}

func bindScope(t *testing.T, store auth.Store, id, principal, role, scope string) {
	t.Helper()
	if err := store.PutBinding(context.Background(), auth.RoleBinding{
		ID: id, PrincipalID: principal, Role: role, Scope: scope,
	}); err != nil {
		t.Fatalf("bind %q: %v", id, err)
	}
}

// acmeTwin and globexTwin are twins carrying their ancestor closure.
func acmeTwin(id string) auth.ResourceRef {
	return auth.Resource("twin", id).WithScopes(teamAcmeEng, orgAcme)
}

func globexTwin(id string) auth.ResourceRef {
	return auth.Resource("twin", id).WithScopes(teamGlobexEng, orgGlobex)
}

// The case the design exists for: two tenants whose teams share a display name
// must not share access. Labels carry IDs, so they cannot collide.
func TestTwoOrgsWithTheSameTeamNameAreIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	bindScope(t, store, "b-alice", "alice", "operator", teamAcmeEng)
	bindScope(t, store, "b-bob", "bob", "operator", teamGlobexEng)

	cases := []struct {
		name      string
		principal string
		resource  auth.ResourceRef
		want      bool
	}{
		{name: "alice reaches acme's eng", principal: "alice", resource: acmeTwin("t1"), want: true},
		{name: "alice cannot reach globex's eng", principal: "alice", resource: globexTwin("t2"), want: false},
		{name: "bob reaches globex's eng", principal: "bob", resource: globexTwin("t2"), want: true},
		{name: "bob cannot reach acme's eng", principal: "bob", resource: acmeTwin("t1"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := a.Authorize(ctx, auth.Principal{ID: tc.principal}, "twin:read", tc.resource)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if d.Allowed != tc.want {
				t.Fatalf("allowed = %v, want %v (%s)", d.Allowed, tc.want, d.Reason)
			}
		})
	}
}

// A grant on the org covers everything beneath it, without the authorizer
// walking anything — the closure is already on the resource.
func TestOrgGrantCoversDescendants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	bindScope(t, store, "b-alice", "alice", "operator", orgAcme)

	d, err := a.Authorize(ctx, auth.Principal{ID: "alice"}, "twin:read", acmeTwin("t1"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !d.Allowed {
		t.Fatalf("org grant did not reach a twin in one of its teams: %s", d.Reason)
	}

	d, err = a.Authorize(ctx, auth.Principal{ID: "alice"}, "twin:read", globexTwin("t2"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if d.Allowed {
		t.Fatal("an acme org grant reached a globex twin")
	}
}

// Deny still wins across levels, and needs no new precedence rule: it is the
// same "any covering binding, deny wins" the module already documents.
func TestDenyAtOrgBeatsAllowAtTeam(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	bindScope(t, store, "b-allow", "alice", "operator", teamAcmeEng)
	bindScope(t, store, "b-deny", "alice", "quarantined", orgAcme)

	d, err := a.Authorize(ctx, auth.Principal{ID: "alice"}, "twin:read", acmeTwin("t1"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if d.Allowed {
		t.Fatal("a deny on the org did not beat an allow on the team inside it")
	}
	if d.Effect != auth.EffectDeny {
		t.Errorf("effect = %q, want deny", d.Effect)
	}
}

// Multi-homing: a resource in two containers at once is what makes cross-tenant
// collaboration possible without a second construct.
func TestSharedProjectReachesAcrossOrgs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	// One acme twin is shared into a project; bob, who lives in globex, is
	// bound as a viewer on that project and nothing else.
	shared := auth.Resource("twin", "shared").WithScopes(teamAcmeEng, orgAcme, projectDelta)
	bindScope(t, store, "b-bob", "bob", "viewer", projectDelta)

	d, err := a.Authorize(ctx, auth.Principal{ID: "bob"}, "twin:read", shared)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !d.Allowed {
		t.Fatalf("the shared twin was not reachable through the project: %s", d.Reason)
	}

	// Sharing is per-resource: acme's other twins stay out of reach.
	d, err = a.Authorize(ctx, auth.Principal{ID: "bob"}, "twin:read", acmeTwin("private"))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if d.Allowed {
		t.Fatal("a project grant leaked to a twin that was never shared")
	}
}

// A resource with no containers behaves exactly as it did before scopes
// existed. This is what makes the change additive rather than a migration.
func TestResourcesWithoutScopesAreUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	legacy := auth.Resource("twin", "legacy")

	cases := []struct {
		scope string
		want  bool
	}{
		{scope: "*", want: true},
		{scope: "twin:*", want: true},
		{scope: "twin:legacy", want: true},
		{scope: "twin:other", want: false},
		{scope: orgAcme, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.scope, func(t *testing.T) {
			store := scopeStore(t)
			a := auth.NewAuthorizer(store)
			bindScope(t, store, "b", "alice", "operator", tc.scope)

			d, err := a.Authorize(ctx, auth.Principal{ID: "alice"}, "twin:read", legacy)
			if err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if d.Allowed != tc.want {
				t.Fatalf("scope %q allowed = %v, want %v", tc.scope, d.Allowed, tc.want)
			}
		})
	}
}

func TestInScope(t *testing.T) {
	t.Parallel()
	twin := acmeTwin("t1")

	cases := []struct {
		pattern string
		want    bool
	}{
		{pattern: "*", want: true},
		{pattern: "twin:*", want: true},
		{pattern: "twin:t1", want: true},
		{pattern: teamAcmeEng, want: true},
		{pattern: orgAcme, want: true},
		{pattern: "team:*", want: true},
		{pattern: teamGlobexEng, want: false},
		{pattern: orgGlobex, want: false},
		{pattern: projectDelta, want: false},
		{pattern: "", want: false},
	}
	for _, tc := range cases {
		if got := twin.InScope(tc.pattern); got != tc.want {
			t.Errorf("InScope(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
	// A ref with no scopes never matches a container.
	if auth.Resource("twin", "t1").InScope(orgAcme) {
		t.Error("a resource with no containers matched one")
	}
}

func TestGrantedScopes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	bindScope(t, store, "b-eng", "alice", "operator", teamAcmeEng)
	bindScope(t, store, "b-proj", "alice", "viewer", projectDelta)
	bindScope(t, store, "b-deny", "alice", "quarantined", orgGlobex)

	got, err := a.GrantedScopes(ctx, "alice", "twin:read")
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if !slices.Equal(got.Allow, []string{projectDelta, teamAcmeEng}) {
		t.Errorf("Allow = %v, want the team and the project", got.Allow)
	}
	if !slices.Equal(got.Deny, []string{orgGlobex}) {
		t.Errorf("Deny = %v, want the quarantined org", got.Deny)
	}

	// An action nobody granted yields nothing, so a caller can skip the query.
	none, err := a.GrantedScopes(ctx, "alice", "audit:read")
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if !none.Empty() {
		t.Errorf("Allow = %v, want nothing for an ungranted action", none.Allow)
	}
}

func TestGrantedScopesUnrestricted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	bindScope(t, store, "b-all", "alice", "operator", "*")
	got, err := a.GrantedScopes(ctx, "alice", "twin:read")
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if !got.Unrestricted() {
		t.Fatalf("grants = %+v, want unrestricted so listing can skip filtering", got)
	}

	// A deny anywhere means the caller cannot skip filtering, even at "*".
	bindScope(t, store, "b-deny", "alice", "quarantined", orgGlobex)
	got, err = a.GrantedScopes(ctx, "alice", "twin:read")
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if got.Unrestricted() {
		t.Fatal("a principal with a deny was reported unrestricted")
	}
}

// Conditional denies cannot be resolved without the resource, so they are left
// out of the filter and the per-resource check stays authoritative.
func TestGrantedScopesOmitsConditionalDenies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scopeStore(t)
	a := auth.NewAuthorizer(store)

	if err := store.PutRole(ctx, auth.Role{
		Name: "not-yours",
		Policies: []auth.Policy{{
			ID: "deny-others", Action: "twin:*", Resource: "twin:*", Effect: auth.EffectDeny,
			Conditions: []auth.Condition{{Kind: auth.CondOwnerEquals}},
		}},
	}); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	bindScope(t, store, "b-allow", "alice", "operator", orgAcme)
	bindScope(t, store, "b-cond", "alice", "not-yours", orgAcme)

	got, err := a.GrantedScopes(ctx, "alice", "twin:read")
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if len(got.Deny) != 0 {
		t.Fatalf("Deny = %v, want empty — a conditional deny cannot be evaluated at list time", got.Deny)
	}
}

func TestGrantedScopesStoreError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	a := auth.NewAuthorizer(errListBindings{Store: auth.NewMemoryStore(), err: boom})

	if _, err := a.GrantedScopes(context.Background(), "alice", "twin:read"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

type errListBindings struct {
	auth.Store
	err error
}

func (e errListBindings) ListBindings(context.Context, string) ([]auth.RoleBinding, error) {
	return nil, e.err
}

func TestScopeLabel(t *testing.T) {
	t.Parallel()
	if got := auth.ScopeLabel("team", "t_7f2a"); got != "team:t_7f2a" {
		t.Fatalf("ScopeLabel = %q", got)
	}
}

func TestValidateScopes(t *testing.T) {
	t.Parallel()

	if err := auth.ValidateScopes([]string{orgAcme, teamAcmeEng, projectDelta}); err != nil {
		t.Fatalf("valid labels rejected: %v", err)
	}
	if err := auth.ValidateScopes(nil); err != nil {
		t.Fatalf("empty scopes rejected: %v", err)
	}

	// A label says what a resource is inside; a pattern is the binding's job.
	for _, bad := range []string{"*", "org:*", "", "no-colon", "org:acme:extra*"} {
		if err := auth.ValidateScopes([]string{bad}); !errors.Is(err, auth.ErrInvalidPattern) {
			t.Errorf("ValidateScopes(%q) = %v, want ErrInvalidPattern", bad, err)
		}
	}
}
