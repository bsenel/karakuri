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
type JWTResolver struct {
	Tokens *TokenService

	// AllowQueryParam decides whether a request may carry its token in the
	// query string instead of the Authorization header. This exists for one
	// reason: the browser EventSource API cannot set headers, so an SSE stream
	// has no other way to authenticate.
	//
	// Query strings land in access logs and proxy logs, so this is deliberately
	// opt-in per request rather than blanket-enabled. Nil means header-only.
	AllowQueryParam func(*http.Request) bool
}

// NewJWTResolver returns a resolver that accepts the query-parameter fallback
// only on SSE endpoints.
func NewJWTResolver(tokens *TokenService) *JWTResolver {
	return &JWTResolver{Tokens: tokens, AllowQueryParam: SSEQueryParamPolicy}
}

// SSEQueryParamPolicy allows the access_token query parameter on event-stream
// endpoints only — those whose path ends in "/events", or which explicitly ask
// for text/event-stream.
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
		if !errors.Is(err, ErrNoCredential) || j.AllowQueryParam == nil || !j.AllowQueryParam(r) {
			return Principal{}, err
		}
		token = r.URL.Query().Get(AccessTokenQueryParam)
		if token == "" {
			return Principal{}, ErrNoCredential
		}
	}
	return j.Tokens.Verify(r.Context(), token)
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
