package handler

import (
	"log/slog"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// SSOHandler mounts the federated-login endpoints and turns a successful
// identity-provider round trip into an ordinary Karakuri session.
//
// That last step is the point. Once the provider has said who somebody is, this
// server mints its own access and refresh pair, and everything afterwards —
// rotation, revocation, `/auth/me`, the SSE cookie, `krk` — works exactly as it
// does for a password login. The identity provider is consulted at login and
// not again.
type SSOHandler struct {
	Federation *karakuriauth.Federation
	Tokens     *auth.TokenService
	Cookies    auth.CookieConfig
}

// Config tells an unauthenticated client what kind of login this server offers,
// so a browser can render the right thing before it has any credential.
//
// It is deliberately thin: the provider kind and where to start. Anything more
// would be describing the identity provider to parties who have no business
// knowing about it.
//
// GET /api/v1/auth/sso/config
func (h *SSOHandler) Config(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"provider": h.kind(),
		"enabled":  h.Federation.Enabled(),
		// Password login is always available. It is the break-glass path when
		// the identity provider is unreachable, and a UI needs to know it can
		// still offer the form.
		"password_login": true,
	}
	if h.Federation.Enabled() {
		out["login_url"] = "/api/v1" + karakuriauth.SSOLoginPath
	}
	writeJSON(w, out)
}

func (h *SSOHandler) kind() string {
	if h.Federation == nil || h.Federation.Kind == "" {
		return "bearer"
	}
	return h.Federation.Kind
}

// Login starts the flow at the identity provider.
//
// GET /api/v1/auth/sso/login
func (h *SSOHandler) Login() http.Handler {
	switch {
	case h.Federation.OIDC != nil:
		return h.Federation.OIDC.LoginHandler()
	case h.Federation.SAML != nil:
		return h.Federation.SAML.LoginHandler()
	default:
		return http.HandlerFunc(h.notConfigured)
	}
}

// Callback completes an OIDC flow.
//
// GET /api/v1/auth/sso/callback
func (h *SSOHandler) Callback() http.Handler {
	if h.Federation.OIDC == nil {
		return http.HandlerFunc(h.notConfigured)
	}
	return h.Federation.OIDC.CallbackHandler(h.establishSession, h.reportFailure)
}

// ACS receives a SAML assertion.
//
// POST /api/v1/auth/saml/acs
func (h *SSOHandler) ACS() http.Handler {
	if h.Federation.SAML == nil {
		return http.HandlerFunc(h.notConfigured)
	}
	return h.Federation.SAML.ACSHandler(h.establishSession, h.reportFailure)
}

// Metadata publishes this service provider's SAML metadata for an administrator
// to register with the identity provider.
//
// GET /api/v1/auth/saml/metadata
func (h *SSOHandler) Metadata() http.Handler {
	if h.Federation.SAML == nil {
		return http.HandlerFunc(h.notConfigured)
	}
	return h.Federation.SAML.MetadataHandler()
}

// establishSession mints Karakuri's own tokens for a federated principal and
// sends the browser on.
//
// The tokens go out as httpOnly cookies, never in the response body or a URL
// fragment. A token in a redirect target lands in browser history, proxy logs
// and Referer headers, which is the whole reason the SPA uses cookie mode for
// password login too.
func (h *SSOHandler) establishSession(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	pair, err := h.Tokens.IssueForPrincipal(r.Context(), p.ID)
	if err != nil {
		h.reportFailure(w, r, err)
		return
	}
	h.Cookies.SetSession(w, r, pair)

	slog.Info("federated login",
		"provider", h.kind(),
		"principal", karakuriauth.SanitizeLogValue(p.ID))

	// The destination comes from configuration, never from the request. A
	// "return to" parameter read off an SSO callback is an open redirect, and
	// this is exactly the endpoint an attacker would aim one at.
	http.Redirect(w, r, h.Federation.AbsoluteRedirect(), http.StatusFound)
}

// reportFailure logs the real reason and tells the caller nothing useful.
//
// The distinction matters here more than elsewhere: the reasons a federated
// login fails — an unknown subject, a group that maps to nothing, a signature
// from the wrong issuer — are exactly what somebody probing the endpoint would
// like to learn.
func (h *SSOHandler) reportFailure(w http.ResponseWriter, r *http.Request, err error) {
	slog.Warn("federated login failed",
		"provider", h.kind(),
		"path", karakuriauth.SanitizeLogValue(r.URL.Path),
		"err", err)
	authError(w, http.StatusUnauthorized, "sso_failed", "authentication failed")
}

func (h *SSOHandler) notConfigured(w http.ResponseWriter, _ *http.Request) {
	authError(w, http.StatusNotFound, "sso_not_configured", "no federated identity provider is configured")
}
