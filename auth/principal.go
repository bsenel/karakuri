package auth

import (
	"context"
	"maps"
)

// Kind distinguishes a human login from a machine identity. Service principals
// authenticate with a rotating refresh token and never hold a password.
type Kind string

const (
	KindUser    Kind = "user"
	KindService Kind = "service"
)

// Principal is an authenticated identity. Attrs carry deployment-specific
// metadata (team, region, …) that policy conditions can read; they are never
// interpreted by this package.
type Principal struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	Kind     Kind              `json:"kind,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

// Clone returns a deep copy so callers cannot mutate a stored principal through
// a returned reference.
func (p Principal) Clone() Principal {
	out := p
	if p.Attrs != nil {
		out.Attrs = maps.Clone(p.Attrs)
	}
	return out
}

type principalCtxKey struct{}

// WithPrincipal returns a context carrying p. Authenticate installs it; every
// authorization check downstream reads it back.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFromContext returns the principal installed by WithPrincipal. The
// boolean is false when the request was never authenticated.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}
