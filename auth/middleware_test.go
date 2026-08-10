package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

func TestAuthenticate(t *testing.T) {
	resolver := ResolverFunc(func(r *http.Request) (Principal, error) {
		switch r.Header.Get("X-Who") {
		case "alice":
			return Principal{ID: "alice", Name: "Alice"}, nil
		case "ghost":
			return Principal{ID: "ghost", Disabled: true}, nil
		default:
			return Principal{}, errors.New("no credential")
		}
	})

	var seen Principal
	h := Authenticate(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Who", "alice")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || seen.ID != "alice" {
		t.Fatalf("authenticated request = %d, principal %+v", rec.Code, seen)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body["error"] != "unauthorized" || body["reason"] != "no credential" {
		t.Errorf("401 body = %+v", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Who", "ghost")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(decodeError(t, rec)["reason"], "disabled") {
		t.Fatalf("disabled principal = %d %v", rec.Code, decodeError(t, rec))
	}
}

func TestRequirePermission(t *testing.T) {
	s, a := fixture(t)
	bind(t, s, "b-vera", "vera", "viewer", "*")
	vera := Principal{ID: "vera"}

	allowed := RequirePermission(a, "twin:read", nil)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/twins", nil).WithContext(WithPrincipal(context.Background(), vera))
	rec := httptest.NewRecorder()
	allowed.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed request = %d", rec.Code)
	}

	denied := RequirePermission(a, "twin:update", nil)(okHandler())
	rec = httptest.NewRecorder()
	denied.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/twins", nil).WithContext(WithPrincipal(context.Background(), vera)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied request = %d, want 403", rec.Code)
	}
	if body := decodeError(t, rec); body["error"] != "forbidden" || body["reason"] == "" {
		t.Errorf("403 body = %+v", body)
	}

	// No principal in context — the handler must not run.
	rec = httptest.NewRecorder()
	allowed.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/twins", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request = %d, want 401", rec.Code)
	}
}

func TestRequirePermissionUsesResourceFunc(t *testing.T) {
	s, a := fixture(t)
	bind(t, s, "b-alice", "alice", "owner-only", "*")
	alice := Principal{ID: "alice"}

	// The resourceFn is where ownership enters the decision — here it stands in
	// for a datastore lookup keyed on the URL parameter.
	owners := map[string]string{"t1": "alice", "t2": "bob"}
	resourceFn := func(r *http.Request) ResourceRef {
		id := strings.TrimPrefix(r.URL.Path, "/twins/")
		return Resource("twin", id).WithOwner(owners[id])
	}
	h := RequirePermission(a, "twin:update", resourceFn)(okHandler())

	for path, want := range map[string]int{"/twins/t1": http.StatusOK, "/twins/t2": http.StatusForbidden} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, path, nil).WithContext(WithPrincipal(context.Background(), alice))
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestEnforcerHooks(t *testing.T) {
	s, a := fixture(t)
	bind(t, s, "b-vera", "vera", "viewer", "*")
	vera := Principal{ID: "vera"}

	var denied *Decision
	e := NewEnforcer(a)
	e.OnDeny = func(_ *http.Request, p Principal, d Decision) {
		if p.ID != "vera" {
			t.Errorf("OnDeny principal = %q", p.ID)
		}
		denied = &d
	}
	h := e.Require("twin:update", nil)(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/twins", nil).WithContext(WithPrincipal(context.Background(), vera)))
	if rec.Code != http.StatusForbidden || denied == nil {
		t.Fatalf("code = %d, OnDeny called = %v", rec.Code, denied != nil)
	}
	if denied.Action != "twin:update" || denied.Reason == "" {
		t.Errorf("denial trace = %+v", denied)
	}
}

func TestEnforcerErrorPath(t *testing.T) {
	base, _ := fixture(t)
	bind(t, base, "b-vera", "vera", "viewer", "*")
	boom := errors.New("store down")

	var got error
	e := NewEnforcer(NewAuthorizer(failingStore{Store: base, bindingErr: boom}))
	e.OnError = func(_ *http.Request, err error) { got = err }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/twins", nil).WithContext(WithPrincipal(context.Background(), Principal{ID: "vera"}))
	e.Require("twin:read", nil)(okHandler()).ServeHTTP(rec, req)

	// An authorizer that cannot answer must fail closed, never open.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	if !errors.Is(got, boom) {
		t.Errorf("OnError got %v", got)
	}

	// Without an OnError hook the request is still denied (and logged).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/twins", nil).WithContext(WithPrincipal(context.Background(), Principal{ID: "vera"}))
	NewEnforcer(NewAuthorizer(failingStore{Store: base, bindingErr: boom})).
		Require("twin:read", nil)(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code without hook = %d, want 500", rec.Code)
	}
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body
}
