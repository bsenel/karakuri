package auth_test

import (
	"context"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

func TestEvalRunIsRegisteredWithADescription(t *testing.T) {
	c := karakuriauth.NewCatalog()
	if karakuriauth.ActionEvalRun != "eval:run" {
		t.Fatalf("ActionEvalRun = %q, want eval:run", karakuriauth.ActionEvalRun)
	}
	desc, ok := c.Describe(karakuriauth.ActionEvalRun)
	if !ok {
		t.Fatal("eval:run is not in the catalog")
	}
	if desc == "" {
		t.Fatal("eval:run has no description")
	}
}

// Calibration spends a model call per checkpoint and reads checkpoints across
// twins, so only an administrator holds it — not an operator driving work, and
// not an auditor reading spend.
func TestEvalRunIsAdminOnly(t *testing.T) {
	ctx := context.Background()
	store := extauth.NewMemoryStore()
	for _, r := range karakuriauth.BuiltinRoles() {
		if err := store.PutRole(ctx, r); err != nil {
			t.Fatalf("put role %s: %v", r.Name, err)
		}
	}
	roles := []string{
		karakuriauth.RoleAdmin, karakuriauth.RoleOperator, karakuriauth.RoleContributor,
		karakuriauth.RoleViewer, karakuriauth.RoleAuditor,
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

	authz := extauth.NewAuthorizer(store)
	for _, role := range roles {
		d, err := authz.Authorize(ctx, extauth.Principal{ID: "p-" + role},
			karakuriauth.ActionEvalRun, extauth.Collection("eval"))
		if err != nil {
			t.Fatalf("%s: authorize: %v", role, err)
		}
		want := role == karakuriauth.RoleAdmin
		if d.Allowed != want {
			t.Errorf("%s holds eval:run = %t, want %t (%s)", role, d.Allowed, want, d.Reason)
		}
	}
}

func TestEvalRouteIsGatedOnEvalRun(t *testing.T) {
	for _, r := range karakuriauth.Routes() {
		if r.Pattern == "/eval/calibrate" && r.Method == "POST" {
			if r.Action != karakuriauth.ActionEvalRun || r.Public {
				t.Fatalf("POST /eval/calibrate = %+v, want eval:run and not public", r)
			}
			return
		}
	}
	t.Fatal("POST /eval/calibrate is not in the route table")
}
