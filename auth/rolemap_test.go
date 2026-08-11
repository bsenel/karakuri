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

func TestRoleMapRoles(t *testing.T) {
	t.Parallel()

	m := auth.RoleMap{
		Groups: map[string][]string{
			"karakuri-admins":    {"admin"},
			"karakuri-operators": {"operator"},
			"everyone":           {"viewer", "auditor"},
			"broken":             {""},
		},
	}

	cases := []struct {
		name   string
		groups []string
		want   []string
	}{
		{name: "one group", groups: []string{"karakuri-admins"}, want: []string{"admin"}},
		{name: "several roles from one group", groups: []string{"everyone"}, want: []string{"auditor", "viewer"}},
		{name: "union, sorted and deduplicated", groups: []string{"everyone", "karakuri-operators", "everyone"}, want: []string{"auditor", "operator", "viewer"}},
		{name: "unmapped group grants nothing", groups: []string{"marketing"}, want: nil},
		{name: "no groups", groups: nil, want: nil},
		{name: "empty role names are dropped", groups: []string{"broken"}, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := m.Roles(tc.groups); !slices.Equal(got, tc.want) {
				t.Fatalf("Roles(%v) = %v, want %v", tc.groups, got, tc.want)
			}
		})
	}
}

// An empty Default is the security-relevant default: everybody in a corporate
// directory can authenticate, so matching no group has to mean no access.
func TestRoleMapDefaultIsEmptyUnlessConfigured(t *testing.T) {
	t.Parallel()

	bare := auth.RoleMap{Groups: map[string][]string{"eng": {"operator"}}}
	if got := bare.Roles([]string{"marketing"}); got != nil {
		t.Fatalf("an unmapped group granted %v, want nothing", got)
	}

	withDefault := auth.RoleMap{Groups: map[string][]string{"eng": {"operator"}}, Default: []string{"viewer"}}
	if got := withDefault.Roles([]string{"marketing"}); !slices.Equal(got, []string{"viewer"}) {
		t.Fatalf("Roles = %v, want the configured default", got)
	}
	// The default is a fallback, not an addition: a matched group replaces it.
	if got := withDefault.Roles([]string{"eng"}); !slices.Equal(got, []string{"operator"}) {
		t.Fatalf("Roles = %v, want only the mapped role", got)
	}
}

func TestRoleMapMentions(t *testing.T) {
	t.Parallel()

	m := auth.RoleMap{
		Groups:  map[string][]string{"a": {"operator", "viewer"}, "b": {"viewer"}, "c": {""}},
		Default: []string{"viewer", "auditor"},
	}
	want := []string{"auditor", "operator", "viewer"}
	if got := m.Mentions(); !slices.Equal(got, want) {
		t.Fatalf("Mentions = %v, want %v", got, want)
	}
	if got := (auth.RoleMap{}).Mentions(); got != nil {
		t.Fatalf("Mentions on an empty map = %v, want nil", got)
	}
}
