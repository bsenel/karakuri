package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/oidc"
	karakurisaml "github.com/bsenel/karakuri/auth/saml"
	"github.com/bsenel/karakuri/config"
	crewjam "github.com/crewjam/saml"
)

// SSO route paths, mounted publicly because they are how an unauthenticated
// browser starts and finishes a login.
const (
	SSOLoginPath     = "/auth/sso/login"
	SSOCallbackPath  = "/auth/sso/callback"
	SSOExchangePath  = "/auth/sso/exchange"
	SAMLMetadataPath = "/auth/saml/metadata"
	SAMLACSPath      = "/auth/saml/acs"
)

// ErrNoPublicURL is returned when a federated provider is configured without
// auth.frontend.public_url.
//
// It cannot be inferred from an inbound request. The Host header behind a proxy
// is attacker-controlled, and deriving a redirect target or an assertion
// consumer URL from one is how open redirects and assertion-forwarding attacks
// start. An operator has to say where this server actually lives.
var ErrNoPublicURL = errors.New("auth: auth.frontend.public_url is required when a federated provider is configured")

// Federation is whatever the configured identity provider contributes to the
// auth stack. A deployment with provider "bearer" gets a zero value, and
// everything downstream is written so that costs nothing.
type Federation struct {
	// Kind is the configured provider: bearer, oidc or saml.
	Kind string

	// OIDC and SAML are populated for their respective kinds and nil otherwise.
	OIDC *oidc.Provider
	SAML *karakurisaml.Provider

	// Provisioner is what turns an asserted identity into a local principal.
	Provisioner *auth.Provisioner

	// LoginRedirect is where a browser lands after a successful login.
	LoginRedirect string

	// Sealer signs the short-lived values that travel through a browser: the
	// login-flow state each provider keeps, and the handoff code the CLI login
	// uses. It carries the key derived from the JWT signing material, so every
	// replica agrees without a second secret to distribute.
	Sealer auth.Sealer
}

// Enabled reports whether a federated provider is configured at all.
func (f *Federation) Enabled() bool {
	return f != nil && (f.OIDC != nil || f.SAML != nil)
}

// Resolver returns the per-request resolver this provider contributes, or nil.
//
// Only OIDC has one. A SAML assertion is a one-time login artifact, not a
// credential a client can present on every request, so SAML contributes login
// handlers and nothing else — see the auth/saml package doc.
func (f *Federation) Resolver() auth.TokenResolver {
	if f != nil && f.OIDC != nil {
		return f.OIDC
	}
	return nil
}

// BuildFederation constructs the configured identity provider.
//
// Every failure is fatal at startup, deliberately. A deployment that asked for
// SSO and silently did not get it is worse off than one that never configured
// it: nobody can log in, and the reason is invisible until someone reads a log
// line that was never written.
// containers may be nil, which is what a deployment with no tenancy tree has.
// A role map that names one is then a configuration error rather than a silent
// grant over everything.
func BuildFederation(ctx context.Context, cfg *config.Config, store auth.Store, containers ContainerLookup) (*Federation, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Auth.Provider))
	if kind == "" {
		kind = config.AuthProviderBearer
	}
	f := &Federation{Kind: kind, LoginRedirect: cfg.Auth.Frontend.LoginRedirect}

	switch kind {
	case config.AuthProviderBearer:
		return f, nil
	case config.AuthProviderOIDC, config.AuthProviderSAML:
	default:
		return nil, fmt.Errorf("auth.provider %q is not one of bearer, oidc, saml", cfg.Auth.Provider)
	}

	public := strings.TrimSuffix(strings.TrimSpace(cfg.Auth.Frontend.PublicURL), "/")
	if public == "" {
		return nil, ErrNoPublicURL
	}

	stateKey, err := flowStateKey(cfg.Auth.JWT)
	if err != nil {
		return nil, err
	}

	// Container names resolve to IDs once, here. After this point nothing in
	// the auth path has ever seen a display name.
	roles, err := BuildRoleMap(ctx, cfg.Auth.RoleMap, containers)
	if err != nil {
		return nil, fmt.Errorf("auth.role_map: %w", err)
	}

	prefix := kind // "oidc" or "saml", which is exactly the namespace we want
	provisioner := &auth.Provisioner{
		Store:  store,
		Prefix: prefix,
		Roles:  roles,
	}
	// A group mapped to a role nobody registered would deny every login that
	// matched it. Catch the typo here, where an operator is reading startup
	// output, rather than in somebody's browser.
	if err := provisioner.Validate(ctx); err != nil {
		return nil, fmt.Errorf("auth.role_map: %w", err)
	}
	f.Provisioner = provisioner
	f.Sealer = auth.Sealer{Key: stateKey}

	if kind == config.AuthProviderOIDC {
		if err := f.buildOIDC(ctx, cfg, public, stateKey); err != nil {
			return nil, err
		}
		return f, nil
	}
	if err := f.buildSAML(ctx, cfg, public, stateKey); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Federation) buildOIDC(ctx context.Context, cfg *config.Config, public string, stateKey []byte) error {
	redirect := cfg.Auth.OIDC.RedirectURL
	if redirect == "" {
		redirect = public + "/api/v1" + SSOCallbackPath
	}

	provider, err := oidc.New(ctx, oidc.Config{
		IssuerURL:         cfg.Auth.OIDC.IssuerURL,
		ClientID:          cfg.Auth.OIDC.ClientID,
		ClientSecret:      cfg.Auth.OIDC.ClientSecret,
		RedirectURL:       redirect,
		Scopes:            cfg.Auth.OIDC.Scopes,
		GroupsClaim:       auth.ClaimPath(cfg.Auth.OIDC.GroupsClaim),
		EmailClaim:        auth.ClaimPath(cfg.Auth.OIDC.EmailClaim),
		NameClaim:         auth.ClaimPath(cfg.Auth.OIDC.NameClaim),
		StateKey:          stateKey,
		InsecureAllowHTTP: cfg.Auth.Cookies.InsecureAllowHTTP,
	}, f.Provisioner)
	if err != nil {
		return fmt.Errorf("auth.oidc: %w", err)
	}
	f.OIDC = provider
	return nil
}

func (f *Federation) buildSAML(ctx context.Context, cfg *config.Config, public string, stateKey []byte) error {
	metadata, err := loadIDPMetadata(ctx, cfg.Auth.SAML)
	if err != nil {
		return fmt.Errorf("auth.saml: %w", err)
	}

	entityID := cfg.Auth.SAML.EntityID
	if entityID == "" {
		entityID = public + "/api/v1" + SAMLMetadataPath
	}
	provider, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          entityID,
		MetadataURL:       public + "/api/v1" + SAMLMetadataPath,
		ACSURL:            public + "/api/v1" + SAMLACSPath,
		IDPMetadata:       metadata,
		RoleAttribute:     cfg.Auth.SAML.RoleAttribute,
		EmailAttribute:    cfg.Auth.SAML.EmailAttribute,
		NameAttribute:     cfg.Auth.SAML.NameAttribute,
		StateKey:          stateKey,
		InsecureAllowHTTP: cfg.Auth.Cookies.InsecureAllowHTTP,
		AllowIDPInitiated: cfg.Auth.SAML.AllowIDPInitiated,
	}, f.Provisioner)
	if err != nil {
		return fmt.Errorf("auth.saml: %w", err)
	}
	f.SAML = provider
	return nil
}

// maxMetadataBytes bounds a metadata document. It is another party's output
// fetched over the network, and a startup path should not be a way to make this
// process allocate without limit.
const maxMetadataBytes = 1 << 20

func loadIDPMetadata(ctx context.Context, cfg config.AuthSAMLConfig) (*crewjam.EntityDescriptor, error) {
	switch {
	case cfg.IDPMetadataFile != "":
		body, err := os.ReadFile(cfg.IDPMetadataFile)
		if err != nil {
			return nil, fmt.Errorf("read idp_metadata_file: %w", err)
		}
		return parseIDPMetadata(body)
	case cfg.IDPMetadataURL != "":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.IDPMetadataURL, nil)
		if err != nil {
			return nil, fmt.Errorf("idp_metadata_url: %w", err)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch idp_metadata_url: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch idp_metadata_url: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
		if err != nil {
			return nil, fmt.Errorf("read idp_metadata_url: %w", err)
		}
		return parseIDPMetadata(body)
	default:
		return nil, errors.New("set idp_metadata_url or idp_metadata_file")
	}
}

func parseIDPMetadata(body []byte) (*crewjam.EntityDescriptor, error) {
	var descriptor crewjam.EntityDescriptor
	if err := xml.Unmarshal(body, &descriptor); err != nil {
		// Some providers publish an EntitiesDescriptor wrapping one or more
		// EntityDescriptors. crewjam parses the inner shape, so unwrap.
		var entities crewjam.EntitiesDescriptor
		if inner := xml.Unmarshal(body, &entities); inner != nil {
			return nil, fmt.Errorf("parse metadata: %w", err)
		}
		if len(entities.EntityDescriptors) == 0 {
			return nil, errors.New("metadata contains no EntityDescriptor")
		}
		return &entities.EntityDescriptors[0], nil
	}
	if len(descriptor.IDPSSODescriptors) == 0 {
		return nil, errors.New("metadata declares no IDPSSODescriptor")
	}
	return &descriptor, nil
}

// flowStateKeyLabel domain-separates the derived key from the signing key it
// comes from, so the two can never be confused for one another.
const flowStateKeyLabel = "karakuri/federation/flow-state/v1"

// flowStateKey derives the key that signs login-flow cookies from the JWT
// signing material.
//
// Deriving rather than configuring means every replica agrees without a second
// secret to distribute, and one fewer setting to forget — a flow key that
// differs between replicas produces logins that fail intermittently behind a
// load balancer, which is the worst way to find out.
//
// Rotating the JWT signing key rotates this too, which invalidates logins that
// are mid-flight at that moment. That is a few seconds of "please try again"
// during a key rotation, and it is the right trade against a separate secret
// nobody rotates.
func flowStateKey(cfg config.JWTConfig) ([]byte, error) {
	material := signingMaterial(cfg)
	if len(material) == 0 {
		return nil, fmt.Errorf("%w: federated login needs signing material to derive its flow key from", ErrNoSigningKey)
	}
	mac := hmac.New(sha256.New, material)
	mac.Write([]byte(flowStateKeyLabel))
	return mac.Sum(nil), nil
}

// signingMaterial returns bytes unique to this deployment's signing key.
//
// It prefers the key flagged active, and falls back to the first key that has
// any material, mirroring how NewKeyring picks a signer.
func signingMaterial(cfg config.JWTConfig) []byte {
	var fallback []byte
	for _, k := range cfg.Keys {
		material := keyMaterial(k)
		if len(material) == 0 {
			continue
		}
		if k.Active {
			return material
		}
		if fallback == nil {
			fallback = material
		}
	}
	return fallback
}

func keyMaterial(k config.JWTKeyConfig) []byte {
	if k.Secret != "" {
		return []byte(k.Secret)
	}
	if k.PrivateKeyFile != "" {
		// The file's contents rather than its path: two deployments sharing a
		// path but not a key must not share a flow key.
		if body, err := os.ReadFile(k.PrivateKeyFile); err == nil {
			return body
		}
	}
	return nil
}

// AbsoluteRedirect resolves the post-login landing page against the configured
// public URL.
//
// It deliberately ignores anything the request carries. A "return to" parameter
// taken from the query string is an open redirect, and an SSO callback is
// precisely where an attacker would like one.
func (f *Federation) AbsoluteRedirect() string {
	target := f.LoginRedirect
	if target == "" {
		return "/"
	}
	if u, err := url.Parse(target); err == nil && u.IsAbs() {
		return target
	}
	if !strings.HasPrefix(target, "/") {
		return "/" + target
	}
	return target
}
