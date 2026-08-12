package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/go-chi/chi/v5"
)

type stubScopes struct {
	labels map[string][]string
	err    error
	calls  int
}

func (s *stubScopes) ScopesOf(_ context.Context, resourceType, resourceID string) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.labels[resourceType+":"+resourceID], nil
}

// routed runs fn the way the enforcer does: after chi has matched, so URL
// parameters resolve.
func routed(t *testing.T, pattern, target string, fn extauth.ResourceFunc) extauth.ResourceRef {
	t.Helper()
	var got extauth.ResourceRef
	r := chi.NewRouter()
	r.Get(pattern, func(_ http.ResponseWriter, req *http.Request) { got = fn(req) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("route %q did not match %q (%d)", pattern, target, rec.Code)
	}
	return got
}

// routedAs is routed with a principal in the context, which is what the
// enforcer has already put there by the time a resource function runs.
func routedAs(t *testing.T, principal extauth.Principal, pattern, target string, fn extauth.ResourceFunc) extauth.ResourceRef {
	t.Helper()
	var got extauth.ResourceRef
	r := chi.NewRouter()
	r.Get(pattern, func(_ http.ResponseWriter, req *http.Request) { got = fn(req) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if principal.ID != "" {
		req = req.WithContext(extauth.WithPrincipal(req.Context(), principal))
	}
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("route %q did not match %q (%d)", pattern, target, rec.Code)
	}
	return got
}

func TestScopedAttachesContainers(t *testing.T) {
	lookup := &stubScopes{labels: map[string][]string{
		"twin:abc": {"team:t_7f2a", "org:o_9c31"},
	}}

	ref := routed(t, "/twins/{id}", "/twins/abc", karakuriauth.Scoped(lookup, karakuriauth.TwinResource))
	if !slices.Equal(ref.Scopes, []string{"team:t_7f2a", "org:o_9c31"}) {
		t.Fatalf("Scopes = %v", ref.Scopes)
	}
	// Which is the point: a binding on the org now covers this twin, and one on
	// another tenant's team does not.
	if !ref.InScope("org:o_9c31") {
		t.Error("an org-scoped binding does not reach a twin inside it")
	}
	if ref.InScope("team:t_be04") {
		t.Error("another tenant's team reaches this twin")
	}
}

func TestScopedComposesWithOwnership(t *testing.T) {
	lookup := &stubScopes{labels: map[string][]string{"twin:abc": {"org:o_9c31"}}}
	owned := karakuriauth.OwnedTwinResource(nil) // no owner lookup: ID only

	ref := routed(t, "/twins/{id}", "/twins/abc", karakuriauth.Scoped(lookup, owned))
	if ref.ID != "abc" || !slices.Equal(ref.Scopes, []string{"org:o_9c31"}) {
		t.Fatalf("ref = %+v, want the id from the inner func and the scopes from the lookup", ref)
	}
}

func TestScopedLeavesUnscopedResourcesAlone(t *testing.T) {
	lookup := &stubScopes{labels: map[string][]string{}}

	ref := routed(t, "/twins/{id}", "/twins/abc", karakuriauth.Scoped(lookup, karakuriauth.TwinResource))
	if len(ref.Scopes) != 0 {
		t.Fatalf("Scopes = %v, want none", ref.Scopes)
	}
	// A twin in no container matches exactly what it matched before containers
	// existed.
	if !ref.InScope("twin:abc") || !ref.InScope("twin:*") || !ref.InScope("*") {
		t.Fatal("an unscoped twin stopped matching its own patterns")
	}
}

// A collection route names no resource, so there is nothing to look up and the
// lookup must not be called — every list request would pay for it.
func TestScopedSkipsCollections(t *testing.T) {
	lookup := &stubScopes{labels: map[string][]string{}}

	ref := routed(t, "/twins", "/twins", karakuriauth.Scoped(lookup, karakuriauth.TwinResource))
	if ref.String() != "twin:*" {
		t.Fatalf("ref = %q, want the collection", ref.String())
	}
	if lookup.calls != 0 {
		t.Fatalf("lookup called %d times on a collection route", lookup.calls)
	}
}

// A failed lookup narrows the decision rather than failing the request: no
// labels means a container-scoped binding does not match, so the failure mode
// is denial, never a resource escaping its tenant.
func TestScopedFailsClosed(t *testing.T) {
	lookup := &stubScopes{err: errors.New("database down")}

	ref := routed(t, "/twins/{id}", "/twins/abc", karakuriauth.Scoped(lookup, karakuriauth.TwinResource))
	if len(ref.Scopes) != 0 {
		t.Fatalf("Scopes = %v, want none after a failed lookup", ref.Scopes)
	}
	if ref.InScope("org:o_9c31") {
		t.Fatal("a failed lookup left the twin covered by an org binding")
	}
}

// A deployment with no container service passes nil, and the resource function
// is handed back untouched rather than wrapped in a lookup that does nothing.
func TestScopedWithNoLookup(t *testing.T) {
	ref := routed(t, "/objectives/{id}", "/objectives/o1",
		karakuriauth.Scoped(nil, karakuriauth.ObjectiveResource))
	if ref.String() != "objective:o1" || len(ref.Scopes) != 0 {
		t.Fatalf("ref = %+v", ref)
	}
}
