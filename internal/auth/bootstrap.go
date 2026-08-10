package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
)

// Seed installs the built-in roles and, on a database that has no principals at
// all, the first administrator.
//
// Both halves are safe on every boot: roles are upserted, and the admin is only
// minted when the principal table is empty. Without this a fresh install would
// have a working API that nobody can authenticate against.
func Seed(ctx context.Context, store auth.Store, tokens *auth.TokenService, catalog *auth.Catalog, cfg config.AuthBootstrapConfig) error {
	roles := BuiltinRoles()

	// Validate before writing: a policy naming an unregistered action, or an
	// inheritance cycle, should fail at boot rather than at the first request
	// that would have matched it.
	if err := auth.ValidateRoles(roles); err != nil {
		return fmt.Errorf("built-in roles: %w", err)
	}
	for _, role := range roles {
		if err := catalog.ValidateRole(role); err != nil {
			return fmt.Errorf("built-in roles: %w", err)
		}
		if err := store.PutRole(ctx, role); err != nil {
			return fmt.Errorf("seed role %q: %w", role.Name, err)
		}
	}

	principals, err := store.ListPrincipals(ctx)
	if err != nil {
		return fmt.Errorf("list principals: %w", err)
	}
	if len(principals) > 0 {
		return nil
	}
	return seedAdmin(ctx, store, tokens, cfg)
}

// ErrNoBootstrapPassword is returned when a database has no principals and no
// bootstrap password is configured. It is fatal at startup for the same reason
// a missing signing key is: continuing would leave a server nobody can log in
// to, and the alternative — generating a password and logging it — writes a
// live credential into the log stream.
var ErrNoBootstrapPassword = errors.New("auth: no bootstrap password configured")

func seedAdmin(ctx context.Context, store auth.Store, tokens *auth.TokenService, cfg config.AuthBootstrapConfig) error {
	id := cfg.AdminID
	if id == "" {
		id = "admin"
	}

	// Deliberately not generated-and-logged. Karakuri fans its logs out to
	// Datadog, Loki, Elasticsearch and CloudWatch (Phase 12), so "logged once
	// at WARN" means "copied to every configured log sink" — which is exactly
	// where a live administrator credential must not end up.
	password := os.Getenv(cfg.EnvVar)
	if password == "" {
		return fmt.Errorf("%w: this database has no principals, so %s must be set to create the first administrator (%q)",
			ErrNoBootstrapPassword, cfg.EnvVar, id)
	}

	if err := store.PutPrincipal(ctx, auth.Principal{ID: id, Name: "Bootstrap administrator", Kind: auth.KindUser}); err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	if err := store.PutBinding(ctx, auth.RoleBinding{
		ID:          "bootstrap-admin",
		PrincipalID: id,
		Role:        RoleAdmin,
		Scope:       "*",
	}); err != nil {
		return fmt.Errorf("bind bootstrap admin: %w", err)
	}
	if err := tokens.SetPassword(ctx, id, password); err != nil {
		return fmt.Errorf("set bootstrap admin password: %w", err)
	}

	slog.Info("created bootstrap administrator", "id", id)
	return nil
}
