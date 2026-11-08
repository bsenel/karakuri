package auth_test

import (
	"net/http"
	"testing"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// Phase 37 step 4: POST /stop-all. The owner approved this permission design:
// the route sits behind authentication and is not public, but it carries no
// single blanket permission. Each run it reaches is checked on its own, inside
// the handler, with loop:stop on each loop and objective:pause on each
// objective, against the same resource refs POST /loops/{id}/stop and
// POST /objectives/{id}/pause use. That is what lets a caller scoped to one
// objective pause that one only.
//
// The route table has no field for "checked per run in the handler"; a Route
// carries one Action. Read before writing this: the routes that are
// authenticated but gated on no one action are registered with an empty
// Action and Public false, namely GET /auth/me, POST /auth/revoke and POST /mcp,
// whose comment says the same thing this route needs ("a JSON-RPC body names
// the tool, and each tool demands the same action as the route answering the
// same question"). The integration RBAC walk skips an empty Action and covers
// such routes separately (test/integration/rbac_test.go), and the
// unauthenticated walk still expects 401 from them. Stop-all follows that
// registration.

func TestStopAllPathIsTheRoadmapsName(t *testing.T) {
	if karakuriauth.StopAllPath != "/stop-all" {
		t.Fatalf("StopAllPath = %q, want /stop-all", karakuriauth.StopAllPath)
	}
}

func TestStopAllRouteIsAuthenticatedWithNoBlanketPermission(t *testing.T) {
	var found []karakuriauth.Route
	for _, r := range karakuriauth.Routes() {
		if r.Pattern == karakuriauth.StopAllPath {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		t.Fatalf("%s is not in the route table", karakuriauth.StopAllPath)
	}
	if len(found) != 1 {
		t.Fatalf("%s is in the route table %d times, want once: %+v", karakuriauth.StopAllPath, len(found), found)
	}
	r := found[0]
	if r.Method != http.MethodPost {
		t.Errorf("%s method = %s, want POST", r.Pattern, r.Method)
	}
	if r.Public {
		t.Errorf("%s is public; it must sit behind authentication", r.Pattern)
	}
	// Not loop:stop, not objective:pause, not anything else: a route-level
	// action would either refuse a caller who may stop some runs, or stand in
	// for the per-run checks that make a scoped answer true.
	if r.Action != "" {
		t.Errorf("%s is gated on %q, want no route-level action; each run is checked on its own", r.Pattern, r.Action)
	}
}

// Registered the way the other authenticated-only routes are, so the walks
// that read the table treat it the same.
func TestStopAllIsRegisteredLikeTheOtherAuthenticatedOnlyRoutes(t *testing.T) {
	want := map[string]bool{
		http.MethodGet + " /auth/me":                     true,
		http.MethodPost + " /auth/revoke":                true,
		http.MethodPost + " /mcp":                        true,
		http.MethodPost + " " + karakuriauth.StopAllPath: true,
	}
	for _, r := range karakuriauth.Routes() {
		key := r.Method + " " + r.Pattern
		if !want[key] {
			continue
		}
		if r.Action != "" || r.Public {
			t.Errorf("%s = %+v, want an empty action and not public", key, r)
		}
		delete(want, key)
	}
	for key := range want {
		t.Errorf("%s is not in the route table", key)
	}
}

// Adding stop-all moves no per-run route off its permission: the stop-all
// handler asks the same two actions those routes demand.
func TestStopAllLeavesThePerRunRoutesOnTheirPermissions(t *testing.T) {
	want := map[string]string{
		http.MethodPost + " /loops/{id}/stop":       string(karakuriauth.ActionLoopStop),
		http.MethodPost + " /objectives/{id}/pause": string(karakuriauth.ActionObjectivePause),
	}
	for _, r := range karakuriauth.Routes() {
		key := r.Method + " " + r.Pattern
		if action, ok := want[key]; ok {
			if string(r.Action) != action || r.Public {
				t.Errorf("%s = %+v, want %s and not public", key, r, action)
			}
			delete(want, key)
		}
	}
	for key := range want {
		t.Errorf("%s is not in the route table", key)
	}
}
