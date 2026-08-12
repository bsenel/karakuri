package auth_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func TestListSelectors(t *testing.T) {
	cases := []struct {
		name        string
		grants      extauth.ScopeGrants
		wantAll     bool // visible == nil
		wantVisible storage.ScopeSelector
		wantHidden  storage.ScopeSelector
	}{
		{
			name:    "unrestricted",
			grants:  extauth.ScopeGrants{Allow: []string{"*"}},
			wantAll: true,
		},
		{
			// Every twin by identity is still everything, so the listing is
			// unfiltered — but a deny can still remove some of it, which is why
			// this is not the same code path as holding "*".
			name:    "every resource of the type",
			grants:  extauth.ScopeGrants{Allow: []string{"twin:*"}},
			wantAll: true,
		},
		{
			name:        "one container",
			grants:      extauth.ScopeGrants{Allow: []string{"team:t_7f2a"}},
			wantVisible: storage.ScopeSelector{Labels: []string{"team:t_7f2a"}},
		},
		{
			name:        "one resource by id",
			grants:      extauth.ScopeGrants{Allow: []string{"twin:abc"}},
			wantVisible: storage.ScopeSelector{IDs: []string{"abc"}},
		},
		{
			// A whole kind of container. Carried so a filtered listing is never
			// narrower than the per-row check, which would show a 403 on a list
			// for a row the same principal can fetch by id.
			name:        "a kind of container",
			grants:      extauth.ScopeGrants{Allow: []string{"org:*"}},
			wantVisible: storage.ScopeSelector{LabelPrefixes: []string{"org:"}},
		},
		{
			name: "the mixture a real principal holds",
			grants: extauth.ScopeGrants{
				Allow: []string{"org:o_9c31", "team:t_7f2a", "twin:abc"},
				Deny:  []string{"team:t_be04"},
			},
			wantVisible: storage.ScopeSelector{
				IDs:    []string{"abc"},
				Labels: []string{"org:o_9c31", "team:t_7f2a"},
			},
			wantHidden: storage.ScopeSelector{Labels: []string{"team:t_be04"}},
		},
		{
			// The security-relevant case: no grants means no rows, expressed as
			// an empty selector rather than as "no filter".
			name:        "nothing granted",
			grants:      extauth.ScopeGrants{},
			wantVisible: storage.ScopeSelector{},
		},
		{
			// A deny does not stop the allow from being unrestricted — it
			// subtracts from it.
			name:       "everything, minus one tenant",
			grants:     extauth.ScopeGrants{Allow: []string{"twin:*"}, Deny: []string{"org:o_1111"}},
			wantAll:    true,
			wantHidden: storage.ScopeSelector{Labels: []string{"org:o_1111"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visible, hidden := karakuriauth.ListSelectors(tc.grants, "twin")
			if tc.wantAll {
				if visible != nil {
					t.Fatalf("visible = %+v, want nil (no filtering)", visible)
				}
			} else {
				if visible == nil {
					t.Fatal("visible = nil, want a filter")
				}
				if !sameSelector(*visible, tc.wantVisible) {
					t.Errorf("visible = %+v, want %+v", *visible, tc.wantVisible)
				}
			}
			if !sameSelector(hidden, tc.wantHidden) {
				t.Errorf("hidden = %+v, want %+v", hidden, tc.wantHidden)
			}
		})
	}
}

func sameSelector(a, b storage.ScopeSelector) bool {
	return slices.Equal(a.IDs, b.IDs) &&
		slices.Equal(a.Labels, b.Labels) &&
		slices.Equal(a.LabelPrefixes, b.LabelPrefixes)
}

func TestScopeSelectorEmpty(t *testing.T) {
	if !(storage.ScopeSelector{}).Empty() {
		t.Error("a zero selector is not empty")
	}
	for _, sel := range []storage.ScopeSelector{
		{IDs: []string{"a"}}, {Labels: []string{"org:o_1"}}, {LabelPrefixes: []string{"org:"}},
	} {
		if sel.Empty() {
			t.Errorf("%+v reported itself empty", sel)
		}
	}
}

type stubScopeAuthorizer struct {
	grants extauth.ScopeGrants
	err    error
	action extauth.Action
}

func (s *stubScopeAuthorizer) GrantedScopes(_ context.Context, _ string, action extauth.Action) (extauth.ScopeGrants, error) {
	s.action = action
	return s.grants, s.err
}

// A collection ref is "twin:*", which no container-scoped binding matches. Left
// alone, that means anyone bound to a team can read their twins one at a time
// but cannot call GET /twins at all.
func TestScopedCollection(t *testing.T) {
	principal := extauth.Principal{ID: "alice"}

	cases := []struct {
		name       string
		grants     extauth.ScopeGrants
		wantScopes []string
		covered    string // a binding scope that must now reach the collection
	}{
		{
			name:       "container-scoped",
			grants:     extauth.ScopeGrants{Allow: []string{"team:t_7f2a", "org:o_9c31"}},
			wantScopes: []string{"team:t_7f2a", "org:o_9c31"},
			covered:    "team:t_7f2a",
		},
		{
			// Wildcards already match the collection through the ordinary
			// grammar, so adding them as labels would say nothing new.
			name:       "wildcards are not repeated as labels",
			grants:     extauth.ScopeGrants{Allow: []string{"*", "twin:*", "org:*"}},
			wantScopes: nil,
			covered:    "*",
		},
		{
			// A principal with no grant carries nothing, matches nothing, and
			// is refused exactly as before containers existed.
			name:       "no grants",
			grants:     extauth.ScopeGrants{},
			wantScopes: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &stubScopeAuthorizer{grants: tc.grants}
			fn := karakuriauth.ScopedCollection(a, karakuriauth.ActionTwinRead, karakuriauth.TwinResource)
			ref := routedAs(t, principal, "/twins", "/twins", fn)

			if !slices.Equal(ref.Scopes, tc.wantScopes) {
				t.Fatalf("Scopes = %v, want %v", ref.Scopes, tc.wantScopes)
			}
			if tc.covered != "" && !ref.InScope(tc.covered) {
				t.Errorf("a binding scoped %q still does not reach the collection", tc.covered)
			}
			// Another tenant's team never reaches it.
			if ref.InScope("team:t_be04") {
				t.Error("a binding from another tenant reaches the collection")
			}
		})
	}
}

func TestScopedCollectionLeavesNamedResourcesAlone(t *testing.T) {
	a := &stubScopeAuthorizer{grants: extauth.ScopeGrants{Allow: []string{"team:t_7f2a"}}}
	fn := karakuriauth.ScopedCollection(a, karakuriauth.ActionTwinRead, karakuriauth.TwinResource)

	ref := routedAs(t, extauth.Principal{ID: "alice"}, "/twins/{id}", "/twins/abc", fn)
	if len(ref.Scopes) != 0 {
		t.Fatalf("Scopes = %v — a named resource gets its own containers, not the caller's", ref.Scopes)
	}
}

// Every failure mode leaves the collection unscoped, so only an unscoped
// binding reaches it — the behaviour from before containers existed.
func TestScopedCollectionFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		a         karakuriauth.ScopeAuthorizer
		principal extauth.Principal
	}{
		{"no authorizer", nil, extauth.Principal{ID: "alice"}},
		{"no principal", &stubScopeAuthorizer{grants: extauth.ScopeGrants{Allow: []string{"team:t_7f2a"}}}, extauth.Principal{}},
		{"authorizer error", &stubScopeAuthorizer{err: errors.New("store down")}, extauth.Principal{ID: "alice"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := karakuriauth.ScopedCollection(tc.a, karakuriauth.ActionTwinRead, karakuriauth.TwinResource)
			ref := routedAs(t, tc.principal, "/twins", "/twins", fn)
			if len(ref.Scopes) != 0 {
				t.Fatalf("Scopes = %v, want none", ref.Scopes)
			}
			if ref.InScope("team:t_7f2a") {
				t.Error("a container-scoped binding reached the collection anyway")
			}
		})
	}
}

func TestListFor(t *testing.T) {
	ctx := context.Background()

	a := &stubScopeAuthorizer{grants: extauth.ScopeGrants{Allow: []string{"org:o_9c31"}}}
	visible, _, err := karakuriauth.ListFor(ctx, a, "alice", karakuriauth.ActionTwinRead, "twin")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	if visible == nil || !slices.Equal(visible.Labels, []string{"org:o_9c31"}) {
		t.Fatalf("visible = %+v", visible)
	}
	if a.action != karakuriauth.ActionTwinRead {
		t.Errorf("asked about %q, want the read action", a.action)
	}

	// An authorizer that cannot answer must not produce an unfiltered list.
	broken := &stubScopeAuthorizer{err: errors.New("store down")}
	if _, _, err := karakuriauth.ListFor(ctx, broken, "alice", karakuriauth.ActionTwinRead, "twin"); err == nil {
		t.Fatal("a failing authorizer produced a filter instead of an error")
	}

	// No authorizer and no principal are internal callers, not anonymous ones:
	// every API route authenticates before a handler runs, so filtering here
	// would silently empty those listings.
	for _, tc := range []struct {
		name string
		a    karakuriauth.ScopeAuthorizer
		id   string
	}{
		{"no authorizer", nil, "alice"},
		{"no principal", a, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			visible, hidden, err := karakuriauth.ListFor(ctx, tc.a, tc.id, karakuriauth.ActionTwinRead, "twin")
			if err != nil {
				t.Fatalf("ListFor: %v", err)
			}
			if visible != nil || !hidden.Empty() {
				t.Fatalf("visible = %+v, hidden = %+v, want no filtering", visible, hidden)
			}
		})
	}
}
