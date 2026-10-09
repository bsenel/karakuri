// Package delegation issues the credential a CLI agent holds for one delegated
// action: read-only, scoped to the objective's twin, valid no longer than the
// action's timeout and revoked when the action ends (Phase 34 step 3).
package delegation

import (
	"context"
	"errors"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/jwt"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// EventKindIssued is the tool-event kind recorded when a credential is issued.
const EventKindIssued = "delegation_credential_issued"

// ErrNotImplemented is returned until the issuer is implemented.
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

// Issue mints a credential for a principal holding a read-only binding scoped
// to twinID, expiring no later than timeout from now.
func (i *Issuer) Issue(ctx context.Context, twinID string, timeout time.Duration) (Credential, error) {
	return Credential{}, ErrNotImplemented
}

// Revoke removes the credential's principal and bindings, so its token is
// refused from then on.
func (i *Issuer) Revoke(ctx context.Context, c Credential) error {
	return ErrNotImplemented
}
