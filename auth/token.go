package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/bsenel/karakuri/auth/jwt"
)

var (
	// ErrInvalidRefreshToken is returned when a refresh token does not resolve
	// to a stored record.
	ErrInvalidRefreshToken = errors.New("auth: refresh token is not valid")

	// ErrRefreshTokenExpired is returned when a refresh token is past its TTL.
	ErrRefreshTokenExpired = errors.New("auth: refresh token has expired")

	// ErrRefreshTokenRevoked is returned when a refresh token's family has been
	// revoked.
	ErrRefreshTokenRevoked = errors.New("auth: refresh token has been revoked")

	// ErrRefreshTokenReuse is returned when an already-spent refresh token is
	// presented again. Because tokens rotate on every use, a second use means
	// either a replay or a stolen token racing the legitimate holder — either
	// way the whole family is revoked and both parties must re-authenticate.
	ErrRefreshTokenReuse = errors.New("auth: refresh token reuse detected")
)

// Token types carried in the "typ" claim.
const (
	TokenTypeAccess = "access"
)

// RefreshToken is one link in a rotation chain. Tokens are stored only as a
// hash; FamilyID ties every descendant of an original login together so reuse
// detection can revoke the entire lineage at once.
type RefreshToken struct {
	ID          string     `json:"id"`
	FamilyID    string     `json:"family_id"`
	ParentID    string     `json:"parent_id,omitempty"`
	PrincipalID string     `json:"principal_id"`
	TokenHash   string     `json:"-"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// Spent reports whether the token has already been exchanged.
func (t RefreshToken) Spent() bool { return t.UsedAt != nil }

// CredentialStore persists login material and refresh-token lineage. It is
// separate from Store because a deployment may keep credentials somewhere the
// authorization model does not live — an external IdP, a secrets manager.
type CredentialStore interface {
	GetCredential(ctx context.Context, principalID string) (Credential, error)
	PutCredential(ctx context.Context, c Credential) error
	DeleteCredential(ctx context.Context, principalID string) error

	PutRefreshToken(ctx context.Context, t RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error)

	// SpendRefreshToken atomically marks a token used, reporting false if it
	// was already spent. This has to be a compare-and-set rather than a plain
	// write: two clients racing on the same token must produce exactly one
	// winner, and the loser must be indistinguishable from a replay.
	SpendRefreshToken(ctx context.Context, id string, at time.Time) (bool, error)

	RevokeRefreshFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeRefreshTokensForPrincipal(ctx context.Context, principalID string, at time.Time) error
}

// TokenConfig parameterises token issuance.
type TokenConfig struct {
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func (c TokenConfig) withDefaults() TokenConfig {
	if c.AccessTTL <= 0 {
		c.AccessTTL = 15 * time.Minute
	}
	if c.RefreshTTL <= 0 {
		c.RefreshTTL = 30 * 24 * time.Hour
	}
	return c
}

// TokenPair is what a login or a refresh returns.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenService mints and rotates credentials.
//
// Access tokens are short-lived JWTs, verified statelessly. Refresh tokens are
// opaque, stored hashed, and rotate on every use: exchanging one spends it and
// mints a child. Presenting a spent token is treated as compromise and revokes
// its entire family (OAuth 2.1 BCP §4.14.2).
type TokenService struct {
	store   Store
	creds   CredentialStore
	keys    *jwt.Keyring
	cfg     TokenConfig
	policy  PasswordPolicy
	nowFunc func() time.Time
}

// NewTokenService wires a token service. Passing a nil keyring is a programming
// error — there is no insecure default signing key.
func NewTokenService(store Store, creds CredentialStore, keys *jwt.Keyring, cfg TokenConfig) (*TokenService, error) {
	if store == nil || creds == nil {
		return nil, errors.New("auth: token service needs a store and a credential store")
	}
	if keys == nil {
		return nil, jwt.ErrNoActiveKey
	}
	if _, err := keys.Active(); err != nil {
		return nil, err
	}
	return &TokenService{
		store:  store,
		creds:  creds,
		keys:   keys,
		cfg:    cfg.withDefaults(),
		policy: DefaultPasswordPolicy,
	}, nil
}

// WithPasswordPolicy overrides the hashing cost. Intended for tests.
func (s *TokenService) WithPasswordPolicy(p PasswordPolicy) *TokenService {
	s.policy = p
	return s
}

// WithClock overrides the time source. Intended for tests.
func (s *TokenService) WithClock(now func() time.Time) *TokenService {
	s.nowFunc = now
	return s
}

func (s *TokenService) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc().UTC()
	}
	return time.Now().UTC()
}

// SetPassword hashes and stores a principal's password.
func (s *TokenService) SetPassword(ctx context.Context, principalID, password string) error {
	if _, err := s.store.GetPrincipal(ctx, principalID); err != nil {
		return err
	}
	hash, err := s.policy.Hash(password)
	if err != nil {
		return err
	}
	return s.creds.PutCredential(ctx, Credential{PrincipalID: principalID, PasswordHash: hash, UpdatedAt: s.now()})
}

// IssueForPassword authenticates a principal by password and starts a new
// refresh-token family.
func (s *TokenService) IssueForPassword(ctx context.Context, principalID, password string) (TokenPair, error) {
	principal, err := s.store.GetPrincipal(ctx, principalID)
	if err != nil {
		// Do not leak whether the principal exists.
		return TokenPair{}, ErrBadCredential
	}
	if principal.Disabled {
		return TokenPair{}, ErrBadCredential
	}
	cred, err := s.creds.GetCredential(ctx, principalID)
	if err != nil || cred.PasswordHash == "" {
		return TokenPair{}, ErrBadCredential
	}
	if err := VerifyPassword(cred.PasswordHash, password); err != nil {
		return TokenPair{}, ErrBadCredential
	}
	return s.issue(ctx, principal, "", "")
}

// IssueForPrincipal mints a pair without a password check. It is the path an
// administrator takes when creating a service account, and the caller is
// responsible for having authorized that action.
func (s *TokenService) IssueForPrincipal(ctx context.Context, principalID string) (TokenPair, error) {
	principal, err := s.store.GetPrincipal(ctx, principalID)
	if err != nil {
		return TokenPair{}, err
	}
	if principal.Disabled {
		return TokenPair{}, fmt.Errorf("%w: %q", ErrPrincipalDisabled, principalID)
	}
	return s.issue(ctx, principal, "", "")
}

// IssueForRefresh exchanges a refresh token for a fresh pair, rotating it.
//
// The presented token is spent in the process. Presenting a spent token again
// revokes the whole family: rotation means a legitimate holder never replays,
// so a replay is evidence the token leaked.
func (s *TokenService) IssueForRefresh(ctx context.Context, raw string) (TokenPair, error) {
	now := s.now()
	rt, err := s.creds.GetRefreshTokenByHash(ctx, HashToken(raw))
	if err != nil {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	if rt.RevokedAt != nil {
		return TokenPair{}, ErrRefreshTokenRevoked
	}
	if rt.Spent() {
		return TokenPair{}, s.reuseDetected(ctx, rt.FamilyID, now)
	}
	if now.After(rt.ExpiresAt) {
		return TokenPair{}, ErrRefreshTokenExpired
	}

	principal, err := s.store.GetPrincipal(ctx, rt.PrincipalID)
	if err != nil {
		return TokenPair{}, err
	}
	if principal.Disabled {
		return TokenPair{}, fmt.Errorf("%w: %q", ErrPrincipalDisabled, principal.ID)
	}

	// Spending is the serialization point. Losing this race is indistinguishable
	// from a replay — and must be treated the same, because from the server's
	// side it is the same observation: one token, two exchanges.
	spent, err := s.creds.SpendRefreshToken(ctx, rt.ID, now)
	if err != nil {
		return TokenPair{}, fmt.Errorf("spend refresh token: %w", err)
	}
	if !spent {
		return TokenPair{}, s.reuseDetected(ctx, rt.FamilyID, now)
	}
	return s.issue(ctx, principal, rt.FamilyID, rt.ID)
}

// reuseDetected revokes a compromised lineage and returns the sentinel.
func (s *TokenService) reuseDetected(ctx context.Context, familyID string, at time.Time) error {
	if err := s.creds.RevokeRefreshFamily(ctx, familyID, at); err != nil {
		return fmt.Errorf("revoke reused family %q: %w", familyID, err)
	}
	return ErrRefreshTokenReuse
}

// Revoke invalidates a refresh token's entire family.
func (s *TokenService) Revoke(ctx context.Context, raw string) error {
	rt, err := s.creds.GetRefreshTokenByHash(ctx, HashToken(raw))
	if err != nil {
		return ErrInvalidRefreshToken
	}
	return s.creds.RevokeRefreshFamily(ctx, rt.FamilyID, s.now())
}

// RevokeAllForPrincipal invalidates every refresh token a principal holds. It is
// what "log out everywhere" and "this account is compromised" both call.
func (s *TokenService) RevokeAllForPrincipal(ctx context.Context, principalID string) error {
	return s.creds.RevokeRefreshTokensForPrincipal(ctx, principalID, s.now())
}

// Verify parses and validates an access token, then reloads the principal from
// the store.
//
// The reload is deliberate: claims are a snapshot from issue time, so a
// principal disabled a minute ago would otherwise keep working until its access
// token expired.
func (s *TokenService) Verify(ctx context.Context, token string) (Principal, error) {
	claims, err := jwt.Parse(token, s.keys, jwt.Validation{
		Issuer:   s.cfg.Issuer,
		Audience: s.cfg.Audience,
		Type:     TokenTypeAccess,
		Now:      s.nowFunc,
	})
	if err != nil {
		return Principal{}, err
	}
	principal, err := s.store.GetPrincipal(ctx, claims.Subject)
	if err != nil {
		return Principal{}, err
	}
	if principal.Disabled {
		return Principal{}, fmt.Errorf("%w: %q", ErrPrincipalDisabled, principal.ID)
	}
	return principal, nil
}

// issue mints an access token plus the next refresh token in a family. Passing
// an empty familyID starts a new one.
func (s *TokenService) issue(ctx context.Context, p Principal, familyID, parentID string) (TokenPair, error) {
	now := s.now()

	roles, scopes, err := s.bindingSummary(ctx, p.ID)
	if err != nil {
		return TokenPair{}, err
	}

	jti, err := newOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}
	key, err := s.keys.Active()
	if err != nil {
		return TokenPair{}, err
	}
	expiresAt := now.Add(s.cfg.AccessTTL)
	access, err := jwt.Sign(jwt.Claims{
		Issuer:    s.cfg.Issuer,
		Subject:   p.ID,
		Audience:  s.cfg.Audience,
		ExpiresAt: expiresAt.Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		ID:        jti,
		Type:      TokenTypeAccess,
		Name:      p.Name,
		Kind:      string(p.Kind),
		Roles:     roles,
		Scopes:    scopes,
		Attrs:     p.Attrs,
	}, key)
	if err != nil {
		return TokenPair{}, err
	}

	rawRefresh, err := newOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}
	id, err := newOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}
	if familyID == "" {
		familyID = id
	}
	if err := s.creds.PutRefreshToken(ctx, RefreshToken{
		ID:          id,
		FamilyID:    familyID,
		ParentID:    parentID,
		PrincipalID: p.ID,
		TokenHash:   HashToken(rawRefresh),
		IssuedAt:    now,
		ExpiresAt:   now.Add(s.cfg.RefreshTTL),
	}); err != nil {
		return TokenPair{}, fmt.Errorf("persist refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		ExpiresAt:    expiresAt,
	}, nil
}

// bindingSummary collects the advisory roles and scopes stamped into a token.
func (s *TokenService) bindingSummary(ctx context.Context, principalID string) (roles, scopes []string, err error) {
	bindings, err := s.store.ListBindings(ctx, principalID)
	if err != nil {
		return nil, nil, fmt.Errorf("list bindings for %q: %w", principalID, err)
	}
	for _, b := range bindings {
		if !slices.Contains(roles, b.Role) {
			roles = append(roles, b.Role)
		}
		if scope := b.EffectiveScope(); !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	slices.Sort(roles)
	slices.Sort(scopes)
	return roles, scopes, nil
}
