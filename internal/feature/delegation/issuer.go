// Package delegation issues the credential a CLI agent holds for one delegated
// action: read-only, scoped to the objective's twin, valid no longer than the
// action's timeout and revoked when the action ends (Phase 34 step 3).
package delegation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/jwt"
	internalauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// EventKindIssued is the tool-event kind recorded when a credential is issued.
const EventKindIssued = "delegation_credential_issued"

// ErrNotImplemented is kept for the tests that assert a refusal is a real one.
var ErrNotImplemented = errors.New("delegation: not implemented")

// eventRecorder is the slice of storage.StorageAdapter the issuer audits to.
type eventRecorder interface {
	SaveToolEvent(ctx context.Context, e storage.ToolEvent) error
}

// Credential is what one delegation holds. Token is the bearer token; the
// other fields are its scope and are safe to record.
type Credential struct {
	Token       string
	PrincipalID string
	TwinID      string
	ExpiresAt   time.Time
}

// Issuer mints and revokes delegation credentials. Everything it reads or
// writes comes in through the constructor.
type Issuer struct {
	store   auth.Store
	keys    *jwt.Keyring
	cfg     auth.TokenConfig
	events  eventRecorder
	nowFunc func() time.Time
}

// NewIssuer wires an issuer. cfg carries the issuer and audience the API's
// TokenService verifies against; its TTLs are not used.
func NewIssuer(store auth.Store, keys *jwt.Keyring, cfg auth.TokenConfig, events eventRecorder) *Issuer {
	return &Issuer{store: store, keys: keys, cfg: cfg, events: events}
}

// WithClock overrides the time source. Intended for tests.
func (i *Issuer) WithClock(now func() time.Time) *Issuer {
	i.nowFunc = now
	return i
}

func (i *Issuer) now() time.Time {
	if i.nowFunc != nil {
		return i.nowFunc()
	}
	return time.Now()
}

// Issue mints a credential for a principal holding a read-only binding scoped
// to twinID, expiring no later than timeout from now.
//
// The token is an access token signed with the API's own keyring, so the API's
// TokenService verifies it like any other. No refresh token and no password
// are stored for the principal: nothing can extend it past the action.
func (i *Issuer) Issue(ctx context.Context, twinID string, timeout time.Duration) (Credential, error) {
	if twinID == "" {
		return Credential{}, errors.New("delegation: a credential needs a twin to be scoped to")
	}
	if timeout <= 0 {
		return Credential{}, fmt.Errorf("delegation: timeout %s is not positive", timeout)
	}
	now := i.now()
	// exp is whole seconds: truncating keeps it at or before now+timeout.
	expiresAt := now.Add(timeout).Truncate(time.Second).UTC()
	if !expiresAt.After(now) {
		return Credential{}, fmt.Errorf("delegation: timeout %s is too short to issue a token for", timeout)
	}
	key, err := i.keys.Active()
	if err != nil {
		return Credential{}, fmt.Errorf("delegation: signing key: %w", err)
	}
	suffix, err := randomHex()
	if err != nil {
		return Credential{}, err
	}
	jti, err := randomHex()
	if err != nil {
		return Credential{}, err
	}

	p := auth.Principal{ID: "delegation-" + suffix, Name: "delegation for twin " + twinID, Kind: auth.KindService}
	scope := auth.ScopeLabel("twin", twinID)
	if err := i.store.PutPrincipal(ctx, p); err != nil {
		return Credential{}, fmt.Errorf("delegation: create principal: %w", err)
	}
	c := Credential{PrincipalID: p.ID, TwinID: twinID, ExpiresAt: expiresAt}
	// From here on a failure leaves a principal behind unless it is removed.
	fail := func(err error) (Credential, error) {
		if rerr := i.Revoke(ctx, c); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return Credential{}, err
	}
	if err := i.store.PutBinding(ctx, auth.RoleBinding{
		ID: p.ID + "-viewer", PrincipalID: p.ID, Role: internalauth.RoleViewer, Scope: scope,
	}); err != nil {
		return fail(fmt.Errorf("delegation: bind principal: %w", err))
	}

	token, err := jwt.Sign(jwt.Claims{
		Issuer:    i.cfg.Issuer,
		Subject:   p.ID,
		Audience:  i.cfg.Audience,
		ExpiresAt: expiresAt.Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		ID:        jti,
		Type:      auth.TokenTypeAccess,
		Name:      p.Name,
		Kind:      string(p.Kind),
		Roles:     []string{internalauth.RoleViewer},
		Scopes:    []string{scope},
	}, key)
	if err != nil {
		return fail(fmt.Errorf("delegation: sign token: %w", err))
	}

	// The audit row carries the scope and the expiry, never the token.
	payload, err := json.Marshal(map[string]any{
		"twin_id":    twinID,
		"scope":      scope,
		"access":     "read_only",
		"role":       internalauth.RoleViewer,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return fail(fmt.Errorf("delegation: audit payload: %w", err))
	}
	if err := i.events.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          "delegation-issued-" + suffix,
		AgentID:     p.ID,
		Success:     true,
		Kind:        EventKindIssued,
		PayloadJSON: string(payload),
	}); err != nil {
		// A credential nobody can audit is not issued.
		return fail(fmt.Errorf("delegation: record issue: %w", err))
	}

	c.Token = token
	return c, nil
}

// Revoke removes the credential's principal and bindings, so its token is
// refused from then on.
func (i *Issuer) Revoke(ctx context.Context, c Credential) error {
	if c.PrincipalID == "" {
		return errors.New("delegation: no principal to revoke")
	}
	bindings, err := i.store.ListBindings(ctx, c.PrincipalID)
	if err != nil {
		return fmt.Errorf("delegation: list bindings: %w", err)
	}
	for _, b := range bindings {
		if err := i.store.DeleteBinding(ctx, b.ID); err != nil {
			return fmt.Errorf("delegation: delete binding %q: %w", b.ID, err)
		}
	}
	if err := i.store.DeletePrincipal(ctx, c.PrincipalID); err != nil {
		return fmt.Errorf("delegation: delete principal: %w", err)
	}
	return nil
}

func randomHex() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("delegation: random id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
