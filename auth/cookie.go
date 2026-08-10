package auth

import (
	"net/http"
	"time"
)

// Cookie-based sessions for browsers.
//
// A single-page app has no safe place to keep a token. Anything reachable from
// JavaScript — localStorage, sessionStorage, a variable — is readable by any
// script that gets injected, and a stolen refresh token is a persistent
// session. An httpOnly cookie is not readable by script at all, and it is
// attached to EventSource connections automatically, which is the other thing
// a browser cannot do with a header.
//
// The trade is CSRF: cookies are sent by the browser whether or not the page
// asked. SameSite=Strict is the mitigation — the browser will not attach these
// cookies to any request initiated from another site — which works because the
// SPA is served from the same origin as the API it calls.
type CookieConfig struct {
	// AccessName holds the short-lived access token. Sent on every API request.
	AccessName string

	// RefreshName holds the rotating refresh token. Give it the narrowest path
	// that still reaches the refresh and revoke endpoints, so it is not
	// attached to ordinary API calls that have no use for it.
	RefreshName string

	// AccessPath and RefreshPath scope which requests carry each cookie.
	AccessPath  string
	RefreshPath string

	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// SetSession writes both cookies from a freshly issued pair.
//
// Call it on login and on every refresh: refresh tokens rotate, so the cookie
// must be replaced in the same response that spends the old one, or the browser
// is left holding a token the server has already marked used.
func (c CookieConfig) SetSession(w http.ResponseWriter, r *http.Request, pair TokenPair) {
	secure := requestIsHTTPS(r)
	http.SetCookie(w, c.cookie(c.AccessName, pair.AccessToken, c.AccessPath, c.AccessTTL, secure))
	http.SetCookie(w, c.cookie(c.RefreshName, pair.RefreshToken, c.RefreshPath, c.RefreshTTL, secure))
}

// ClearSession expires both cookies.
func (c CookieConfig) ClearSession(w http.ResponseWriter, r *http.Request) {
	secure := requestIsHTTPS(r)
	for _, ck := range []struct{ name, path string }{
		{c.AccessName, c.AccessPath},
		{c.RefreshName, c.RefreshPath},
	} {
		expired := c.cookie(ck.name, "", ck.path, 0, secure)
		expired.MaxAge = -1
		expired.Expires = time.Unix(0, 0)
		http.SetCookie(w, expired)
	}
}

// Refresh reads the refresh token a browser sent as a cookie.
func (c CookieConfig) Refresh(r *http.Request) string {
	if c.RefreshName == "" {
		return ""
	}
	ck, err := r.Cookie(c.RefreshName)
	if err != nil {
		return ""
	}
	return ck.Value
}

func (c CookieConfig) cookie(name, value, path string, ttl time.Duration, secure bool) *http.Cookie {
	if path == "" {
		path = "/"
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		// Strict, not Lax: no cross-site request has any business carrying a
		// session for this API, and Strict is what makes the cookies safe
		// without a separate CSRF token.
		SameSite: http.SameSiteStrictMode,
		// Only over TLS when the request arrived over TLS. Hard-coding true
		// would silently break plain-HTTP local development — the browser would
		// discard the cookie and the login would appear to succeed and do
		// nothing.
		Secure: secure,
		MaxAge: int(ttl.Seconds()),
	}
}

// requestIsHTTPS reports whether the request reached the server over TLS,
// including via a terminating proxy that set X-Forwarded-Proto.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
