// Package saml authenticates against a SAML 2.0 identity provider — ADFS,
// Okta, Azure AD, Shibboleth — and turns the assertion into a Karakuri
// principal.
//
// # What this package is responsible for
//
// Three handlers: [Provider.MetadataHandler] publishes this service provider's
// metadata for an administrator to hand the identity provider,
// [Provider.LoginHandler] starts the flow, and [Provider.ACSHandler] receives
// the assertion. The assertion becomes an auth.ExternalIdentity, which an
// auth.Provisioner turns into a principal. Nothing here decides what anybody
// may do.
//
// Signature validation, audience and time-window checks, and the XML itself are
// github.com/crewjam/saml's job. Reimplementing any of that would be a way to
// get it subtly wrong.
//
// # Why there is no TokenResolver
//
// Unlike auth/oidc, this package implements no auth.TokenResolver, and that is
// not an omission. A SAML assertion is a one-time login artifact delivered by
// browser POST, not a credential a client can present on each request: it is
// single-use by design, bound to one recipient URL, and carries a validity
// window measured in minutes. The host mints its own session at the end of the
// flow, and that session authenticates everything afterwards. SAML has no
// machine-to-machine story here because SAML has no machine-to-machine story.
package saml

import (
	"crypto"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
	crewjam "github.com/crewjam/saml"
)

// Default attribute names. Every identity provider spells these differently,
// which is why they are configuration; these are the values that work most
// often out of the box.
const (
	DefaultRoleAttribute  = "groups"
	DefaultEmailAttribute = "email"
	DefaultNameAttribute  = "displayName"

	// DefaultPrefix namespaces principal IDs minted from this provider.
	DefaultPrefix = "saml"
)

var (
	// ErrNoIDPMetadata is returned when no identity-provider metadata is
	// configured. Without it there is no key to verify assertions against, and
	// an unverified assertion is a form somebody filled in.
	ErrNoIDPMetadata = errors.New("saml: identity provider metadata is required")

	// ErrNoACSURL is returned when no assertion-consumer URL is configured. It
	// is baked into the metadata and checked by the provider, so it cannot be
	// inferred from the request — that inference is exactly what an attacker
	// would want.
	ErrNoACSURL = errors.New("saml: assertion consumer service URL is required")

	// ErrNoStateKey is returned when no state-signing key is configured. See
	// auth.Sealer for why it is not generated.
	ErrNoStateKey = errors.New("saml: state signing key is required")

	// ErrNoProvisioner is returned when no provisioner is supplied.
	ErrNoProvisioner = errors.New("saml: provisioner is required")

	// ErrNoSubject is returned when an assertion carries no NameID. There is
	// nothing to key a principal on.
	ErrNoSubject = errors.New("saml: assertion has no subject NameID")
)

// Config parameterises a Provider.
type Config struct {
	// EntityID identifies this service provider to the identity provider.
	// Defaults to MetadataURL.
	EntityID string

	// MetadataURL and ACSURL are this application's own absolute URLs. ACSURL
	// is required: it is published in the metadata and validated by the
	// identity provider against the assertion's Destination.
	MetadataURL string
	ACSURL      string

	// IDPMetadata is the identity provider's parsed metadata — its entity ID,
	// endpoints and signing certificates. Required.
	IDPMetadata *crewjam.EntityDescriptor

	// Key and Certificate sign authentication requests. Both optional: many
	// deployments do not require signed requests, and leaving them unset simply
	// omits the signature.
	Key         crypto.Signer
	Certificate *x509.Certificate

	// RoleAttribute, EmailAttribute and NameAttribute name the assertion
	// attributes to read. Each is matched against both an attribute's Name and
	// its FriendlyName, because providers populate one, the other, or both.
	//
	// Known values worth having to hand: ADFS emits groups as
	// "http://schemas.xmlsoap.org/claims/Group", Azure AD as
	// "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups", Okta as
	// "groups", and Shibboleth as "eduPersonAffiliation".
	RoleAttribute  string
	EmailAttribute string
	NameAttribute  string

	// StateKey signs the cookie carrying the in-flight request ID. Required.
	StateKey []byte

	// StateTTL bounds how long a login may take. Defaults to 10 minutes.
	StateTTL time.Duration

	// CookieName is the flow cookie's name. Defaults to "karakuri_saml_state".
	CookieName string

	// InsecureAllowHTTP drops the Secure attribute from the flow cookie so a
	// plain-HTTP development server works. Never set it in production.
	InsecureAllowHTTP bool

	// AllowIDPInitiated permits logins this application did not start.
	//
	// It is off by default, and turning it on is a real trade: an
	// identity-provider-initiated login has no request of ours to correlate
	// against, so the protection that a response answers a request we actually
	// sent is gone. Some deployments need it — a portal tile that drops the
	// user straight into the application — and for those it is available and
	// documented rather than quietly assumed.
	AllowIDPInitiated bool
}

func (c Config) withDefaults() Config {
	if c.RoleAttribute == "" {
		c.RoleAttribute = DefaultRoleAttribute
	}
	if c.EmailAttribute == "" {
		c.EmailAttribute = DefaultEmailAttribute
	}
	if c.NameAttribute == "" {
		c.NameAttribute = DefaultNameAttribute
	}
	if c.StateTTL <= 0 {
		c.StateTTL = 10 * time.Minute
	}
	if c.CookieName == "" {
		c.CookieName = "karakuri_saml_state"
	}
	if c.EntityID == "" {
		c.EntityID = c.MetadataURL
	}
	return c
}

func (c Config) validate() error {
	switch {
	case c.IDPMetadata == nil:
		return ErrNoIDPMetadata
	case strings.TrimSpace(c.ACSURL) == "":
		return ErrNoACSURL
	case len(c.StateKey) == 0:
		return ErrNoStateKey
	}
	return nil
}

// Provider authenticates against one SAML identity provider.
type Provider struct {
	cfg         Config
	sp          crewjam.ServiceProvider
	provisioner *auth.Provisioner
	sealer      auth.Sealer
}

// New returns a ready Provider.
func New(cfg Config, provisioner *auth.Provisioner) (*Provider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if provisioner == nil {
		return nil, ErrNoProvisioner
	}
	cfg = cfg.withDefaults()

	acs, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("saml: parse ACS URL %q: %w", cfg.ACSURL, err)
	}
	metadata := *acs
	if cfg.MetadataURL != "" {
		parsed, err := url.Parse(cfg.MetadataURL)
		if err != nil {
			return nil, fmt.Errorf("saml: parse metadata URL %q: %w", cfg.MetadataURL, err)
		}
		metadata = *parsed
	}

	return &Provider{
		cfg: cfg,
		sp: crewjam.ServiceProvider{
			EntityID:          cfg.EntityID,
			Key:               cfg.Key,
			Certificate:       cfg.Certificate,
			MetadataURL:       metadata,
			AcsURL:            *acs,
			IDPMetadata:       cfg.IDPMetadata,
			AllowIDPInitiated: cfg.AllowIDPInitiated,
		},
		provisioner: provisioner,
		sealer:      auth.Sealer{Key: cfg.StateKey},
	}, nil
}

// ServiceProvider exposes the underlying crewjam/saml service provider.
//
// It is here so a host can publish metadata through its own machinery or set an
// option this package does not surface, without this package having to grow a
// pass-through field for every one of them.
func (p *Provider) ServiceProvider() *crewjam.ServiceProvider { return &p.sp }

// MetadataHandler serves this service provider's SAML metadata.
//
// It is public by design: metadata is what an administrator hands the identity
// provider to register this application, and it contains only public
// information — entity ID, endpoint URLs, and the signing certificate's public
// half.
func (p *Provider) MetadataHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The error is discarded for the same reason crewjam's own middleware
		// discards it: EntityDescriptor is a struct of strings, times and
		// slices, and encoding/xml does not fail on those. An error nobody can
		// produce is an error path nobody can test.
		body, _ := xml.MarshalIndent(p.sp.Metadata(), "", "  ")
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		_, _ = w.Write(body)
	})
}

// LoginHandler redirects the browser to the identity provider.
//
// The request ID travels back in a signed cookie so the assertion can be
// correlated with a request this application actually made — and so any replica
// can finish a flow another one started.
func (p *Provider) LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// crewjam does not object to an empty binding location: it builds a
		// request addressed to nowhere and the browser is redirected to the
		// current page. Metadata without a redirect endpoint is a
		// misconfiguration, and it should say so.
		destination := p.sp.GetSSOBindingLocation(crewjam.HTTPRedirectBinding)
		if destination == "" {
			http.Error(w, "identity provider advertises no redirect endpoint", http.StatusInternalServerError)
			return
		}

		request, err := p.sp.MakeAuthenticationRequest(
			destination,
			crewjam.HTTPRedirectBinding,
			crewjam.HTTPPostBinding,
		)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		target, err := request.Redirect("", &p.sp)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		cookie, err := p.sealer.Seal(flowState{RequestID: request.ID}, p.cfg.StateTTL)
		if err != nil {
			http.Error(w, "could not start login", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, p.flowCookie(cookie, p.cfg.StateTTL))
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
}

// ACSHandler receives the assertion and hands the principal to onSuccess.
//
// onError may be nil, in which case failures are reported as plain 401s.
func (p *Provider) ACSHandler(
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
		// However this ends, the flow is over.
		http.SetCookie(w, p.flowCookie("", -time.Hour))

		if err := r.ParseForm(); err != nil {
			fail(w, r, fmt.Errorf("saml: parse assertion form: %w", err))
			return
		}

		assertion, err := p.sp.ParseResponse(r, p.possibleRequestIDs(r))
		if err != nil {
			// crewjam wraps the real cause in an InvalidResponseError whose
			// Error() is deliberately vague; the private one is where the
			// reason lives, and a host that logs this wants the reason.
			var invalid *crewjam.InvalidResponseError
			if errors.As(err, &invalid) && invalid.PrivateErr != nil {
				err = invalid.PrivateErr
			}
			fail(w, r, fmt.Errorf("saml: parse assertion: %w", err))
			return
		}

		identity, err := p.identityFromAssertion(assertion)
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

// possibleRequestIDs returns the request IDs an assertion may answer.
//
// Exactly one, normally: the request this browser started. The empty string is
// added only when identity-provider-initiated login is enabled, because that is
// what crewjam takes to mean "unsolicited" — so leaving AllowIDPInitiated off
// really does refuse a response to a request we never sent.
func (p *Provider) possibleRequestIDs(r *http.Request) []string {
	var ids []string
	if cookie, err := r.Cookie(p.cfg.CookieName); err == nil {
		var flow flowState
		if err := p.sealer.Open(cookie.Value, &flow); err == nil && flow.RequestID != "" {
			ids = append(ids, flow.RequestID)
		}
	}
	if p.cfg.AllowIDPInitiated {
		ids = append(ids, "")
	}
	return ids
}

// identityFromAssertion reads the normalised identity out of a verified
// assertion. By this point crewjam has checked the signature, the audience and
// the time window; what is left is reading attributes.
func (p *Provider) identityFromAssertion(a *crewjam.Assertion) (auth.ExternalIdentity, error) {
	if a.Subject == nil || a.Subject.NameID == nil || a.Subject.NameID.Value == "" {
		return auth.ExternalIdentity{}, ErrNoSubject
	}

	identity := auth.ExternalIdentity{
		Issuer:  a.Issuer.Value,
		Subject: a.Subject.NameID.Value,
		Groups:  attributeValues(a, p.cfg.RoleAttribute),
		Email:   firstAttribute(a, p.cfg.EmailAttribute),
		Name:    firstAttribute(a, p.cfg.NameAttribute),
	}
	// A NameID is very often the email address already, and a principal listing
	// reading "saml:alice@example.com" with no email is needlessly worse than
	// one that has it.
	if identity.Email == "" && strings.Contains(identity.Subject, "@") {
		identity.Email = identity.Subject
	}
	return identity, nil
}

// attributeValues collects every value of the named attribute.
//
// The name is matched against both Name and FriendlyName because providers
// populate one, the other, or both — ADFS sends a URI in Name and nothing
// friendly, Okta sends a bare word in Name, Shibboleth sends both.
func attributeValues(a *crewjam.Assertion, name string) []string {
	if name == "" {
		return nil
	}
	var out []string
	for _, statement := range a.AttributeStatements {
		for _, attr := range statement.Attributes {
			if attr.Name != name && attr.FriendlyName != name {
				continue
			}
			for _, value := range attr.Values {
				if v := strings.TrimSpace(value.Value); v != "" && !slices.Contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

func firstAttribute(a *crewjam.Assertion, name string) string {
	if values := attributeValues(a, name); len(values) > 0 {
		return values[0]
	}
	return ""
}

// sameSite picks the cookie policy the POST binding actually requires.
//
// This is the detail that decides whether SAML login works at all. The identity
// provider returns the assertion as an auto-submitting cross-site form POST,
// and browsers do not attach SameSite=Lax cookies to cross-site POSTs — only to
// top-level GET navigations. A Lax flow cookie therefore never arrives, the
// request ID cannot be correlated, and every login is rejected as unsolicited.
// So the cookie is SameSite=None, which requires Secure.
//
// InsecureAllowHTTP is the development escape hatch, and it cannot have both:
// browsers reject SameSite=None without Secure. It falls back to Lax, where
// correlation works only if the identity provider is same-site. That is a
// development-only compromise, which is why the flag says insecure.
func (p *Provider) sameSite() http.SameSite {
	if p.cfg.InsecureAllowHTTP {
		return http.SameSiteLaxMode
	}
	return http.SameSiteNoneMode
}

// flowState is what survives the round trip through the identity provider.
type flowState struct {
	RequestID string `json:"r"`
}

func (p *Provider) flowCookie(value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     p.cfg.CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   !p.cfg.InsecureAllowHTTP,
		SameSite: p.sameSite(),
		MaxAge:   int(maxAge.Seconds()),
	}
}
