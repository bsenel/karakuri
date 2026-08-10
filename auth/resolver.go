package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	// ErrNoCredential is returned when a request carries no credential at all.
	ErrNoCredential = errors.New("auth: no bearer token")

	// ErrMalformedCredential is returned when an Authorization header is
	// present but is not a bearer token. It is kept distinct from
	// ErrNoCredential so a wrong header is never silently ignored in favour of
	// some other credential source — that would hide client bugs.
	ErrMalformedCredential = errors.New("auth: malformed Authorization header")
)

// AccessTokenQueryParam is the query parameter checked when the header is
// absent and the request is eligible (see JWTResolver.AllowQueryParam).
const AccessTokenQueryParam = "access_token"

// JWTResolver authenticates requests from a bearer access token.
//
// Three credential sources, in descending order of preference:
//
//  1. The Authorization header — how API clients authenticate.
//  2. A cookie named CookieName — how browsers should, since an httpOnly
//     cookie is not readable by injected script and is attached to EventSource
//     connections automatically.
//  3. A query parameter, only where AllowQueryParam permits it.
//
// Prefer the cookie over the query parameter for browsers. A token in a URL is
// written to access logs, proxy logs and Referer headers; a token in an
// httpOnly cookie is not, and it solves the same EventSource problem.
type JWTResolver struct {
	Tokens *TokenService

	// CookieName, when set, is read if the Authorization header is absent.
	// Empty disables the cookie source.
	CookieName string

	// AllowQueryParam decides whether a request may carry its token in the
	// query string. It exists for callers that cannot use cookies — a stream
	// consumed cross-origin, say — and is checked only after the cookie.
	//
	// Query strings land in access logs and proxy logs, so this is deliberately
	// opt-in per request rather than blanket-enabled. Nil means never.
	AllowQueryParam func(*http.Request) bool
}

// NewJWTResolver returns a header-or-cookie resolver, with the query-parameter
// fallback disabled. Pass the cookie name your application sets.
func NewJWTResolver(tokens *TokenService, cookieName string) *JWTResolver {
	return &JWTResolver{Tokens: tokens, CookieName: cookieName}
}

// SSEQueryParamPolicy allows the access_token query parameter on event-stream
// endpoints only — those whose path ends in "/events", or which explicitly ask
// for text/event-stream.
//
// Only reach for this if a cookie genuinely cannot work; see JWTResolver.
func SSEQueryParamPolicy(r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, "/events") {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

var _ TokenResolver = (*JWTResolver)(nil)

// Resolve extracts and verifies the request's access token.
func (j *JWTResolver) Resolve(r *http.Request) (Principal, error) {
	token, err := BearerToken(r)
	if err != nil {
		// A header that is present but malformed is a client bug, not a missing
		// credential: fall through to other sources only when there was none.
		if !errors.Is(err, ErrNoCredential) {
			return Principal{}, err
		}
		if token = j.cookieToken(r); token == "" {
			if j.AllowQueryParam == nil || !j.AllowQueryParam(r) {
				return Principal{}, ErrNoCredential
			}
			if token = r.URL.Query().Get(AccessTokenQueryParam); token == "" {
				return Principal{}, ErrNoCredential
			}
		}
	}
	return j.Tokens.Verify(r.Context(), token)
}

func (j *JWTResolver) cookieToken(r *http.Request) string {
	if j.CookieName == "" {
		return ""
	}
	c, err := r.Cookie(j.CookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// BearerToken pulls a token out of the Authorization header.
func BearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", ErrNoCredential
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("%w: expected \"Bearer <token>\"", ErrMalformedCredential)
	}
	return strings.TrimSpace(token), nil
}
