package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

func seedAdmin(ctx context.Context, store auth.Store, tokens *auth.TokenService, cfg config.AuthBootstrapConfig) error {
	id := cfg.AdminID
	if id == "" {
		id = "admin"
	}

	password := os.Getenv(cfg.PasswordEnv)
	generated := false
	if password == "" {
		var err error
		if password, err = randomPassword(); err != nil {
			return fmt.Errorf("generate bootstrap password: %w", err)
		}
		generated = true
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

	if generated {
		// Logged exactly once, on the boot that created the account. There is
		// nowhere else to put it: the operator has no other way in, and storing
		// it anywhere retrievable would defeat hashing it.
		slog.Warn("created bootstrap administrator — change this password now",
			"id", id,
			"password", password,
			"hint", fmt.Sprintf("set %s to choose your own", cfg.PasswordEnv))
	} else {
		slog.Info("created bootstrap administrator from the configured password", "id", id)
	}
	return nil
}

func randomPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
