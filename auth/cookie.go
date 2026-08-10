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

	// InsecureAllowHTTP drops the Secure attribute when a request did not
	// arrive over TLS. Session cookies are Secure by default and this is the
	// only way to turn that off.
	//
	// It exists for plain-HTTP local development, where Secure cookies are
	// discarded by non-browser clients and a login appears to succeed while
	// doing nothing. It must never be set in production: without Secure, a
	// single downgraded request — a stray http:// link, an attacker forcing a
	// navigation — is enough to put the session on the wire in the clear.
	//
	// Browsers do not need it. Chrome and Firefox treat http://localhost as a
	// trustworthy origin and accept Secure cookies there.
	InsecureAllowHTTP bool
}

// SetSession writes both cookies from a freshly issued pair.
//
// Call it on login and on every refresh: refresh tokens rotate, so the cookie
// must be replaced in the same response that spends the old one, or the browser
// is left holding a token the server has already marked used.
func (c CookieConfig) SetSession(w http.ResponseWriter, r *http.Request, pair TokenPair) {
	access := c.cookie(c.AccessName, pair.AccessToken, c.AccessPath, c.AccessTTL)
	refresh := c.cookie(c.RefreshName, pair.RefreshToken, c.RefreshPath, c.RefreshTTL)
	c.relaxForPlainHTTP(r, access, refresh)
	http.SetCookie(w, access)
	http.SetCookie(w, refresh)
}

// ClearSession expires both cookies.
func (c CookieConfig) ClearSession(w http.ResponseWriter, r *http.Request) {
	for _, ck := range []struct{ name, path string }{
		{c.AccessName, c.AccessPath},
		{c.RefreshName, c.RefreshPath},
	} {
		expired := c.cookie(ck.name, "", ck.path, 0)
		expired.MaxAge = -1
		expired.Expires = time.Unix(0, 0)
		c.relaxForPlainHTTP(r, expired)
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

func (c CookieConfig) cookie(name, value, path string, ttl time.Duration) *http.Cookie {
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
		// Secure unconditionally. A session cookie that can travel over plain
		// HTTP is a session cookie an attacker can read off the wire, so the
		// default has to be the safe one and the exception has to be asked for
		// by name — see InsecureAllowHTTP.
		Secure: true,
		MaxAge: int(ttl.Seconds()),
	}
}

// relaxForPlainHTTP is the single, opt-in escape hatch from Secure cookies.
func (c CookieConfig) relaxForPlainHTTP(r *http.Request, cookies ...*http.Cookie) {
	if !c.InsecureAllowHTTP || requestIsHTTPS(r) {
		return
	}
	for _, ck := range cookies {
		ck.Secure = false
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
