// Package oidc authenticates against any OpenID Connect provider — Keycloak,
// Okta, Auth0, Azure AD — and turns the result into a Karakuri principal.
//
// # What this package is responsible for
//
// Two entry points, for the two kinds of client:
//
//   - [Provider.Resolve] implements auth.TokenResolver. A caller that already
//     holds a provider-issued token presents it as a bearer token and is
//     authenticated from it. This is the machine-to-machine path.
//   - [Provider.LoginHandler] and [Provider.CallbackHandler] run the
//     authorization-code flow with PKCE, for humans in browsers.
//
// Both converge on the same place: an auth.ExternalIdentity handed to an
// auth.Provisioner, which upserts the local principal and reconciles its roles.
// Nothing in this package decides what anybody may do — it decides only who
// they are.
//
// # Configuration is claim paths, not conventions
//
// There is no standard OIDC claim for group membership. Keycloak puts realm
// roles under "realm_access.roles", Okta and Auth0 typically use "groups", and
// Azure AD emits object IDs under "groups" unless configured otherwise. So the
// location is configuration ([Config.GroupsClaim]) rather than a constant, and
// mapping those raw names onto roles is the host's RoleMap.
package oidc

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Default claim paths. They are the ones the specification defines (or, for
// groups, the one most providers settled on) and are overridable because
// providers disagree.
const (
	DefaultGroupsClaim = "groups"
	DefaultEmailClaim  = "email"
	DefaultNameClaim   = "name"

	// DefaultPrefix namespaces principal IDs minted from this provider.
	DefaultPrefix = "oidc"
)

// DefaultScopes requests the identity claims this package reads. "openid" is
// mandatory; the other two are what populate email and name.
var DefaultScopes = []string{coreoidc.ScopeOpenID, "profile", "email"}

var (
	// ErrNoIssuer is returned when no issuer URL is configured.
	ErrNoIssuer = errors.New("oidc: issuer URL is required")

	// ErrNoClientID is returned when no client ID is configured. It is also the
	// audience every token is checked against, so there is no safe default.
	ErrNoClientID = errors.New("oidc: client ID is required")

	// ErrNoStateKey is returned when no state-signing key is configured.
	//
	// This is required rather than generated because a generated key lives in
	// one process: behind a load balancer, a login that starts on one replica
	// and returns to another would fail to validate its own state, and would do
	// so intermittently. That is a bad thing to discover in production.
	ErrNoStateKey = errors.New("oidc: state signing key is required")

	// ErrNoProvisioner is returned when no provisioner is supplied. Without one
	// there is nowhere for an authenticated identity to become a principal.
	ErrNoProvisioner = errors.New("oidc: provisioner is required")

	// ErrStateMismatch is returned when a callback's state does not match the
	// one this browser was issued — a cross-site request forgery attempt, or a
	// stale tab.
	ErrStateMismatch = errors.New("oidc: state does not match")

	// ErrNonceMismatch is returned when an ID token's nonce is not the one this
	// login flow asked for, which is how a replayed token is caught.
	ErrNonceMismatch = errors.New("oidc: nonce does not match")

	// ErrNoIDToken is returned when a token response carries no ID token. The
	// access token alone says nothing verifiable about who the user is.
	ErrNoIDToken = errors.New("oidc: token response contained no id_token")
)

// Config parameterises a Provider.
type Config struct {
	// IssuerURL is the provider's base URL, from which discovery derives every
	// endpoint. Required.
	IssuerURL string

	// ClientID is this application's registered client. Required — it is the
	// audience every ID token is verified against.
	ClientID string

	// ClientSecret is used for the code exchange. Public clients using PKCE may
	// leave it empty.
	ClientSecret string

	// RedirectURL is where the provider sends the browser back, and must match
	// what is registered with the provider. Required for the browser flow;
	// unused by Resolve.
	RedirectURL string

	// Scopes requested at login. Defaults to DefaultScopes.
	Scopes []string

	// GroupsClaim, EmailClaim and NameClaim locate those values in the token's
	// claims. Each defaults to the corresponding Default*Claim.
	GroupsClaim auth.ClaimPath
	EmailClaim  auth.ClaimPath
	NameClaim   auth.ClaimPath

	// StateKey signs the cookie carrying a login's state, nonce and PKCE
	// verifier. Required; see ErrNoStateKey.
	StateKey []byte

	// StateTTL bounds how long a login may take. Defaults to 10 minutes.
	StateTTL time.Duration

	// CookieName is the flow cookie's name. Defaults to "karakuri_oidc_state".
	CookieName string

	// InsecureAllowHTTP drops the Secure attribute from the flow cookie so a
	// plain-HTTP development server works. Never set it in production: the
	// cookie carries the PKCE verifier.
	InsecureAllowHTTP bool

	// HTTPClient is used for discovery, JWKS and the code exchange.
	HTTPClient *http.Client
}

func (c Config) withDefaults() Config {
	if len(c.Scopes) == 0 {
		c.Scopes = DefaultScopes
	}
	if c.GroupsClaim == "" {
		c.GroupsClaim = DefaultGroupsClaim
	}
	if c.EmailClaim == "" {
		c.EmailClaim = DefaultEmailClaim
	}
	if c.NameClaim == "" {
		c.NameClaim = DefaultNameClaim
	}
	if c.StateTTL <= 0 {
		c.StateTTL = 10 * time.Minute
	}
	if c.CookieName == "" {
		c.CookieName = "karakuri_oidc_state"
	}
	return c
}

func (c Config) validate() error {
	switch {
	case strings.TrimSpace(c.IssuerURL) == "":
		return ErrNoIssuer
	case strings.TrimSpace(c.ClientID) == "":
		return ErrNoClientID
	case len(c.StateKey) == 0:
		return ErrNoStateKey
	}
	return nil
}

// Provider authenticates against one OpenID Connect issuer.
type Provider struct {
	cfg         Config
	verifier    *coreoidc.IDTokenVerifier
	oauth       *oauth2.Config
	provisioner *auth.Provisioner
	sealer      auth.Sealer
	rand        func(int) (string, error)
}

var _ auth.TokenResolver = (*Provider)(nil)

// New discovers the issuer's configuration and returns a ready Provider.
//
// Discovery happens once, here, so a misconfigured issuer URL fails at startup
// rather than at the first login. The JWKS behind it is fetched lazily and
// cached, and refreshes itself when a token arrives signed by a key it has not
// seen — which is what makes provider key rotation a non-event.
func New(ctx context.Context, cfg Config, provisioner *auth.Provisioner) (*Provider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if provisioner == nil {
		return nil, ErrNoProvisioner
	}
	cfg = cfg.withDefaults()

	if cfg.HTTPClient != nil {
		ctx = coreoidc.ClientContext(ctx, cfg.HTTPClient)
	}
	provider, err := coreoidc.NewProvider(ctx, strings.TrimSuffix(cfg.IssuerURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("oidc: discover %q: %w", cfg.IssuerURL, err)
	}

	return &Provider{
		cfg:      cfg,
		verifier: provider.VerifierContext(ctx, &coreoidc.Config{ClientID: cfg.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       cfg.Scopes,
		},
		provisioner: provisioner,
		sealer:      auth.Sealer{Key: cfg.StateKey},
		rand:        randomString,
	}, nil
}

// Resolve authenticates a request from a provider-issued bearer token.
//
// It is the machine-to-machine path: a client that obtained a token from the
// identity provider directly — a CI job with a client-credentials grant, a
// service mesh — presents it here and is provisioned on arrival exactly as a
// browser login would be.
//
// A request with no Authorization header returns auth.ErrNoCredential, so this
// composes with auth.ChainResolver ahead of or behind a local resolver.
func (p *Provider) Resolve(r *http.Request) (auth.Principal, error) {
	token, err := auth.BearerToken(r)
	if err != nil {
		return auth.Principal{}, err
	}
	identity, err := p.identityFromToken(r.Context(), token, "")
	if err != nil {
		return auth.Principal{}, err
	}
	principal, _, err := p.provisioner.Provision(r.Context(), identity)
	return principal, err
}

// identityFromToken verifies an ID token and reads an identity out of it.
//
// An expected nonce of "" skips the nonce check, which is correct for the
// bearer path — a nonce binds a token to a login flow this server started, and
// there was none.
func (p *Provider) identityFromToken(ctx context.Context, rawToken, expectNonce string) (auth.ExternalIdentity, error) {
	if p.cfg.HTTPClient != nil {
		ctx = coreoidc.ClientContext(ctx, p.cfg.HTTPClient)
	}
	token, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return auth.ExternalIdentity{}, fmt.Errorf("oidc: verify token: %w", err)
	}
	if expectNonce != "" && subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(expectNonce)) != 1 {
		return auth.ExternalIdentity{}, ErrNonceMismatch
	}

	var claims map[string]any
	if err := token.Claims(&claims); err != nil {
		return auth.ExternalIdentity{}, fmt.Errorf("oidc: decode claims: %w", err)
	}
	return auth.ExternalIdentity{
		Issuer:  token.Issuer,
		Subject: token.Subject,
		Email:   p.cfg.EmailClaim.First(claims),
		Name:    p.cfg.NameClaim.First(claims),
		Groups:  p.cfg.GroupsClaim.Strings(claims),
	}, nil
}

// LoginHandler starts the authorization-code flow.
//
// It mints a state, a nonce and a PKCE verifier, stores all three in a signed
// short-lived cookie, and redirects to the provider. The cookie is what lets
// any replica finish a flow another one started — nothing is held in memory
// between the two requests.
func (p *Provider) LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, err := p.rand(32)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		nonce, err := p.rand(32)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		verifier := oauth2.GenerateVerifier()

		cookie, err := p.sealer.Seal(flowState{State: state, Nonce: nonce, Verifier: verifier}, p.cfg.StateTTL)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, p.flowCookie(cookie, p.cfg.StateTTL))

		http.Redirect(w, r, p.oauth.AuthCodeURL(state,
			coreoidc.Nonce(nonce),
			oauth2.S256ChallengeOption(verifier),
		), http.StatusFound)
	})
}

// CallbackHandler completes the flow and hands the principal to onSuccess.
//
// The callback is the host's chance to do whatever a login means to it —
// Karakuri mints its own session so that everything downstream keeps using the
// tokens it already understands, and the provider is not consulted again until
// the next login.
//
// onError may be nil, in which case failures are reported as plain 401s.
func (p *Provider) CallbackHandler(
	onSuccess func(http.ResponseWriter, *http.Request, auth.Principal),
	onError func(http.ResponseWriter, *http.Request, error),
) http.Handler {
	fail := func(w http.ResponseWriter, r *http.Request, err error) {
		if onError != nil {
			onError(w, r, err)
			return
		}
		http.Error(w, "authentication failed", http.StatusUnauthorized)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// However this ends, the flow is over: the cookie carries a live PKCE
		// verifier and must not outlive its one use.
		http.SetCookie(w, p.flowCookie("", -time.Hour))

		if desc := r.URL.Query().Get("error"); desc != "" {
			fail(w, r, fmt.Errorf("oidc: provider refused: %s", desc))
			return
		}

		cookie, err := r.Cookie(p.cfg.CookieName)
		if err != nil {
			fail(w, r, fmt.Errorf("oidc: no login in progress: %w", err))
			return
		}
		var flow flowState
		if err := p.sealer.Open(cookie.Value, &flow); err != nil {
			fail(w, r, fmt.Errorf("oidc: login cookie: %w", err))
			return
		}
		if subtle.ConstantTimeCompare([]byte(flow.State), []byte(r.URL.Query().Get("state"))) != 1 {
			fail(w, r, ErrStateMismatch)
			return
		}

		ctx := r.Context()
		if p.cfg.HTTPClient != nil {
			ctx = context.WithValue(ctx, oauth2.HTTPClient, p.cfg.HTTPClient)
		}
		token, err := p.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(flow.Verifier))
		if err != nil {
			fail(w, r, fmt.Errorf("oidc: exchange code: %w", err))
			return
		}
		rawID, ok := token.Extra("id_token").(string)
		if !ok || rawID == "" {
			fail(w, r, ErrNoIDToken)
			return
		}

		identity, err := p.identityFromToken(r.Context(), rawID, flow.Nonce)
		if err != nil {
			fail(w, r, err)
			return
		}
		principal, _, err := p.provisioner.Provision(r.Context(), identity)
		if err != nil {
			fail(w, r, err)
			return
		}
		onSuccess(w, r, principal)
	})
}

func (p *Provider) flowCookie(value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:  p.cfg.CookieName,
		Value: value,
		Path:  "/",
		// The cookie holds the PKCE verifier and the nonce, so script must not
		// be able to read it, and it must not travel to another site. Lax
		// rather than Strict because the provider redirects across sites into
		// the callback and Strict would withhold it exactly then.
		HttpOnly: true,
		Secure:   !p.cfg.InsecureAllowHTTP,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	}
}

// flowState is what survives between the redirect out and the redirect back.
//
// It rides in a cookie sealed by auth.Sealer rather than in memory, so a
// replica that did not start a login can still finish it.
type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
}
