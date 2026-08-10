package auth

import (
	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
)

// Cookie names and paths for browser sessions.
//
// The refresh cookie is scoped to the auth endpoints that spend it, so the
// long-lived credential is not attached to ordinary API calls that have no use
// for it. The access cookie has to reach every /api/v1 route, including the SSE
// streams, so its path is the API root.
const (
	AccessCookieName  = "karakuri_access"
	RefreshCookieName = "karakuri_refresh"

	accessCookiePath  = "/api/v1"
	refreshCookiePath = "/api/v1/auth"
)

// CookieConfig builds the browser session cookie settings from configuration,
// so cookie lifetimes track the token lifetimes rather than drifting from them.
//
// Everything security-relevant is fixed here — HttpOnly, SameSite=Strict and
// Secure are not configurable — except the one documented development escape
// hatch, which an operator has to ask for by name.
func CookieConfig(cfg config.AuthConfig) auth.CookieConfig {
	return auth.CookieConfig{
		AccessName:        AccessCookieName,
		RefreshName:       RefreshCookieName,
		AccessPath:        accessCookiePath,
		RefreshPath:       refreshCookiePath,
		AccessTTL:         cfg.JWT.AccessTTLDuration(),
		RefreshTTL:        cfg.JWT.RefreshTTLDuration(),
		InsecureAllowHTTP: cfg.Cookies.InsecureAllowHTTP,
	}
}
