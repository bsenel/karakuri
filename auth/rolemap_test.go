package auth_test

import (
	"slices"
	"testing"

	"github.com/bsenel/karakuri/auth"
)

func TestClaimPathStrings(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"groups":       []any{"eng", "ops"},
		"typed":        []string{"already", "strings"},
		"single":       "solo",
		"empty":        "",
		"number":       42,
		"mixed":        []any{"keep", 7, "", "also"},
		"realm_access": map[string]any{"roles": []any{"offline_access"}},
		"nested":       map[string]any{"deep": map[string]any{"leaf": "found"}},
	}

	cases := []struct {
		name string
		path auth.ClaimPath
		want []string
	}{
		{name: "list of any", path: "groups", want: []string{"eng", "ops"}},
		{name: "list of string", path: "typed", want: []string{"already", "strings"}},
		{name: "single string", path: "single", want: []string{"solo"}},
		{name: "empty string yields nothing", path: "empty", want: nil},
		{name: "non-string scalar yields nothing", path: "number", want: nil},
		{name: "mixed list keeps only strings", path: "mixed", want: []string{"keep", "also"}},
		{name: "keycloak shape", path: "realm_access.roles", want: []string{"offline_access"}},
		{name: "two levels deep", path: "nested.deep.leaf", want: []string{"found"}},
		{name: "missing key", path: "absent", want: nil},
		{name: "missing branch", path: "absent.deeper", want: nil},
		{name: "descending through a scalar", path: "single.more", want: nil},
		{name: "empty path", path: "", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.path.Strings(claims); !slices.Equal(got, tc.want) {
				t.Fatalf("Strings = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClaimPathStringsNilClaims(t *testing.T) {
	t.Parallel()
	if got := auth.ClaimPath("groups").Strings(nil); got != nil {
		t.Fatalf("Strings on nil claims = %v, want nil", got)
	}
}

func TestClaimPathFirst(t *testing.T) {
	t.Parallel()

	claims := map[string]any{"email": "alice@example.com", "groups": []any{"eng", "ops"}}
	if got := auth.ClaimPath("email").First(claims); got != "alice@example.com" {
		t.Fatalf("First = %q", got)
	}
	if got := auth.ClaimPath("groups").First(claims); got != "eng" {
		t.Fatalf("First of a list = %q, want the first element", got)
	}
	if got := auth.ClaimPath("absent").First(claims); got != "" {
		t.Fatalf("First of a missing claim = %q, want empty", got)
	}
}

func role(name string) auth.RoleGrant { return auth.RoleGrant{Role: name, Scope: "*"} }

func scoped(name, scope string) auth.RoleGrant { return auth.RoleGrant{Role: name, Scope: scope} }

func TestRoleMapGrants(t *testing.T) {
	t.Parallel()

	m := auth.RoleMap{
		Groups: map[string][]auth.RoleGrant{
			"karakuri-admins":    {{Role: "admin"}},
			"karakuri-operators": {{Role: "operator"}},
			"everyone":           {{Role: "viewer"}, {Role: "auditor"}},
			"broken":             {{Role: ""}},
			"acme-engineers":     {{Role: "operator", Scope: "team:t_7f2a"}},
			"globex-engineers":   {{Role: "operator", Scope: "team:t_be04"}},
		},
	}

	cases := []struct {
		name   string
		groups []string
		want   []auth.RoleGrant
	}{
		{name: "one group", groups: []string{"karakuri-admins"}, want: []auth.RoleGrant{role("admin")}},
		{name: "several roles from one group", groups: []string{"everyone"}, want: []auth.RoleGrant{role("auditor"), role("viewer")}},
		{
			name:   "union, sorted and deduplicated",
			groups: []string{"everyone", "karakuri-operators", "everyone"},
			want:   []auth.RoleGrant{role("auditor"), role("operator"), role("viewer")},
		},
		{name: "unmapped group grants nothing", groups: []string{"marketing"}, want: nil},
		{name: "no groups", groups: nil, want: nil},
		{name: "empty role names are dropped", groups: []string{"broken"}, want: nil},
		{
			name:   "an unset scope means everything",
			groups: []string{"karakuri-operators"},
			want:   []auth.RoleGrant{scoped("operator", "*")},
		},
		{
			// The same role in two tenants is two grants. Collapsing them on
			// the role name would either widen one or drop the other.
			name:   "the same role at two scopes stays two grants",
			groups: []string{"acme-engineers", "globex-engineers"},
			want:   []auth.RoleGrant{scoped("operator", "team:t_7f2a"), scoped("operator", "team:t_be04")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := m.Grants(tc.groups); !slices.Equal(got, tc.want) {
				t.Fatalf("Grants(%v) = %v, want %v", tc.groups, got, tc.want)
			}
		})
	}
}

// An empty Default is the security-relevant default: everybody in a corporate
// directory can authenticate, so matching no group has to mean no access.
func TestRoleMapDefaultIsEmptyUnlessConfigured(t *testing.T) {
	t.Parallel()

	bare := auth.RoleMap{Groups: map[string][]auth.RoleGrant{"eng": {{Role: "operator"}}}}
	if got := bare.Grants([]string{"marketing"}); got != nil {
		t.Fatalf("an unmapped group granted %v, want nothing", got)
	}

	withDefault := auth.RoleMap{
		Groups:  map[string][]auth.RoleGrant{"eng": {{Role: "operator"}}},
		Default: []auth.RoleGrant{{Role: "viewer"}},
	}
	if got := withDefault.Grants([]string{"marketing"}); !slices.Equal(got, []auth.RoleGrant{role("viewer")}) {
		t.Fatalf("Grants = %v, want the configured default", got)
	}
	// The default is a fallback, not an addition: a matched group replaces it.
	if got := withDefault.Grants([]string{"eng"}); !slices.Equal(got, []auth.RoleGrant{role("operator")}) {
		t.Fatalf("Grants = %v, want only the mapped role", got)
	}
}

func TestRoleMapMentions(t *testing.T) {
	t.Parallel()

	m := auth.RoleMap{
		Groups: map[string][]auth.RoleGrant{
			"a": {{Role: "operator", Scope: "org:o_1"}, {Role: "viewer"}},
			"b": {{Role: "viewer"}},
			"c": {{Role: ""}},
		},
		Default: []auth.RoleGrant{{Role: "viewer"}, {Role: "auditor"}},
	}
	want := []string{"auditor", "operator", "viewer"}
	if got := m.Mentions(); !slices.Equal(got, want) {
		t.Fatalf("Mentions = %v, want %v", got, want)
	}
	if got := (auth.RoleMap{}).Mentions(); got != nil {
		t.Fatalf("Mentions on an empty map = %v, want nil", got)
	}
}

// Scopes is what a host application checks against its own container tree at
// boot. The unrestricted scope is excluded because there is nothing to look up.
func TestRoleMapScopes(t *testing.T) {
	t.Parallel()

	m := auth.RoleMap{
		Groups: map[string][]auth.RoleGrant{
			"a": {{Role: "operator", Scope: "team:t_7f2a"}, {Role: "admin", Scope: "org:o_9c31"}},
			"b": {{Role: "viewer"}},
			"c": {{Role: "viewer", Scope: "team:t_7f2a"}},
		},
		Default: []auth.RoleGrant{{Role: "auditor", Scope: "org:o_9c31"}},
	}
	want := []string{"org:o_9c31", "team:t_7f2a"}
	if got := m.Scopes(); !slices.Equal(got, want) {
		t.Fatalf("Scopes = %v, want %v", got, want)
	}
	if got := (auth.RoleMap{}).Scopes(); got != nil {
		t.Fatalf("Scopes on an empty map = %v, want nil", got)
	}
}
