package auth_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// Phase 37 step 3: whoever may start a one-shot loop may stop it. The roles
// were read in full before this was written: loop:start is held by operator
// and contributor by name and by admin through its wildcard; viewer and
// auditor hold neither it nor loop:resume. Contributor lacks loop:resume and
// still gets loop:stop, because halting is the easier permission to hold, the
// same reasoning roles.go gives for contributor-objective-pause.

func TestLoopStopIsRegisteredBesideLoopResume(t *testing.T) {
	if karakuriauth.ActionLoopStop != "loop:stop" {
		t.Fatalf("ActionLoopStop = %q, want loop:stop", karakuriauth.ActionLoopStop)
	}
	c := karakuriauth.NewCatalog()
	desc, ok := c.Describe(karakuriauth.ActionLoopStop)
	if !ok {
		t.Fatal("loop:stop is not in the catalog")
	}
	if desc == "" {
		t.Fatal("loop:stop has no description")
	}
	if _, ok := c.Describe(karakuriauth.ActionLoopResume); !ok {
		t.Fatal("loop:resume is no longer in the catalog")
	}
}

func TestLoopStopRouteIsGatedOnLoopStop(t *testing.T) {
	for _, r := range karakuriauth.Routes() {
		if r.Pattern == "/loops/{id}/stop" && r.Method == http.MethodPost {
			if r.Action != karakuriauth.ActionLoopStop || r.Public {
				t.Fatalf("POST /loops/{id}/stop = %+v, want loop:stop and not public", r)
			}
			return
		}
	}
	t.Fatal("POST /loops/{id}/stop is not in the route table")
}

// The other loop routes keep the permissions they had: adding a stop does not
// move start, status or resume onto it.
func TestLoopRoutesKeepTheirPermissions(t *testing.T) {
	want := map[string]extauth.Action{
		http.MethodPost + " /loops":             karakuriauth.ActionLoopStart,
		http.MethodGet + " /loops/{id}/status":  karakuriauth.ActionLoopRead,
		http.MethodPost + " /loops/{id}/resume": karakuriauth.ActionLoopResume,
	}
	for _, r := range karakuriauth.Routes() {
		key := r.Method + " " + r.Pattern
		if action, ok := want[key]; ok {
			if r.Action != action {
				t.Errorf("%s = %q, want %q", key, r.Action, action)
			}
			delete(want, key)
		}
	}
	for key := range want {
		t.Errorf("%s is not in the route table", key)
	}
}

// builtinAuthorizer seeds the built-in roles and one principal bound to each,
// named "p-<role>".
func builtinAuthorizer(t *testing.T) (*extauth.StoreAuthorizer, []string) {
	t.Helper()
	ctx := context.Background()
	store := extauth.NewMemoryStore()
	var roles []string
	for _, r := range karakuriauth.BuiltinRoles() {
		if err := store.PutRole(ctx, r); err != nil {
			t.Fatalf("put role %s: %v", r.Name, err)
		}
		roles = append(roles, r.Name)
	}
	for _, role := range roles {
		p := extauth.Principal{ID: "p-" + role}
		if err := store.PutPrincipal(ctx, p); err != nil {
			t.Fatalf("put principal: %v", err)
		}
		if err := store.PutBinding(ctx, extauth.RoleBinding{ID: "b-" + role, PrincipalID: p.ID, Role: role}); err != nil {
			t.Fatalf("put binding: %v", err)
		}
	}
	return extauth.NewAuthorizer(store), roles
}

func allowed(t *testing.T, authz *extauth.StoreAuthorizer,role string, action extauth.Action, ref extauth.ResourceRef) bool {
	t.Helper()
	d, err := authz.Authorize(context.Background(), extauth.Principal{ID: "p-" + role}, action, ref)
	if err != nil {
		t.Fatalf("%s %s: authorize: %v", role, action, err)
	}
	return d.Allowed
}

// Every role that may start a loop may stop it, and no role that may not start
// one gains the stop. Checked on a named loop and on the collection, so a
// policy scoped to "loop:*" alone would not pass.
func TestLoopStopIsHeldExactlyByTheRolesThatHoldLoopStart(t *testing.T) {
	authz, roles := builtinAuthorizer(t)
	for _, ref := range []extauth.ResourceRef{extauth.Resource("loop", "loop-1"), extauth.Collection("loop")} {
		for _, role := range roles {
			start := allowed(t, authz, role, karakuriauth.ActionLoopStart, ref)
			stop := allowed(t, authz, role, karakuriauth.ActionLoopStop, ref)
			if start != stop {
				t.Errorf("%s on %s: loop:start = %t, loop:stop = %t; want the same", role, ref.String(), start, stop)
			}
		}
	}
}

// Pinned by name as well, so the test above cannot pass by taking loop:start
// away from somebody.
func TestLoopStopByRole(t *testing.T) {
	authz, _ := builtinAuthorizer(t)
	want := map[string]bool{
		karakuriauth.RoleAdmin:       true,
		karakuriauth.RoleOperator:    true,
		karakuriauth.RoleContributor: true,
		karakuriauth.RoleViewer:      false,
		karakuriauth.RoleAuditor:     false,
	}
	ref := extauth.Resource("loop", "loop-1")
	for role, w := range want {
		if got := allowed(t, authz, role, karakuriauth.ActionLoopStop, ref); got != w {
			t.Errorf("%s holds loop:stop = %t, want %t", role, got, w)
		}
		if got := allowed(t, authz, role, karakuriauth.ActionLoopStart, ref); got != w {
			t.Errorf("%s holds loop:start = %t, want %t", role, got, w)
		}
	}
}

// The stop is granted inside the existing roles; no role is added for it.
func TestLoopStopAddsNoRole(t *testing.T) {
	var names []string
	for _, r := range karakuriauth.BuiltinRoles() {
		names = append(names, r.Name)
	}
	slices.Sort(names)
	want := []string{
		karakuriauth.RoleAdmin, karakuriauth.RoleAuditor, karakuriauth.RoleContributor,
		karakuriauth.RoleOperator, karakuriauth.RoleViewer,
	}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("built-in roles = %v, want exactly %v", names, want)
	}
}
