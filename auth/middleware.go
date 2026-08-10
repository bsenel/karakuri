package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// TokenResolver turns an inbound request into an authenticated principal.
// Implementations decide where the credential lives (Authorization header,
// query parameter, mTLS peer certificate, …) and how it is verified.
type TokenResolver interface {
	Resolve(r *http.Request) (Principal, error)
}

// ResolverFunc adapts a plain function to TokenResolver.
type ResolverFunc func(r *http.Request) (Principal, error)

func (f ResolverFunc) Resolve(r *http.Request) (Principal, error) { return f(r) }

// ResourceFunc derives the resource a request targets. It runs after routing,
// so it can read URL parameters, and may consult a datastore to populate Owner
// for ownership conditions.
type ResourceFunc func(r *http.Request) ResourceRef

// Enforcer wires an Authorizer into HTTP middleware and exposes the hooks a
// host application needs — auditing denials, reporting internal failures —
// without widening the middleware signature for callers that want neither.
type Enforcer struct {
	Authorizer Authorizer

	// OnDeny, when set, is called for every denied request. Karakuri uses it to
	// write an audit row carrying the decision trace.
	OnDeny func(r *http.Request, p Principal, d Decision)

	// OnError, when set, is called when the authorizer itself fails (a store
	// outage, a malformed role). Such requests are denied with a 500, never
	// allowed through.
	OnError func(r *http.Request, err error)
}

// NewEnforcer returns an Enforcer with no hooks installed.
func NewEnforcer(a Authorizer) *Enforcer { return &Enforcer{Authorizer: a} }

// Authenticate resolves the request's credential and installs the resulting
// principal in the context. Requests that fail to resolve are rejected with 401
// — authorization middleware downstream can then assume a principal exists.
func Authenticate(tr TokenResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := tr.Resolve(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
				return
			}
			if p.Disabled {
				writeError(w, http.StatusUnauthorized, "unauthorized", "principal is disabled")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// Require returns middleware enforcing one permission. resourceFn may be nil,
// in which case the action's own type is used as a collection reference
// (e.g. "twin:create" targets "twin:*").
func (e *Enforcer) Require(action Action, resourceFn ResourceFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", ErrNoPrincipal.Error())
				return
			}

			res := Collection(action.Type())
			if resourceFn != nil {
				res = resourceFn(r)
			}

			d, err := e.Authorizer.Authorize(r.Context(), p, action, res)
			if err != nil {
				if e.OnError != nil {
					e.OnError(r, err)
				} else {
					slog.Error("authorization failed", "action", action, "resource", res.String(), "error", err)
				}
				writeError(w, http.StatusInternalServerError, "authorization_error", "authorization could not be evaluated")
				return
			}
			if !d.Allowed {
				if e.OnDeny != nil {
					e.OnDeny(r, p, d)
				}
				writeError(w, http.StatusForbidden, "forbidden", d.Reason)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission is the no-hooks form of Enforcer.Require, for callers that
// just want the check.
func RequirePermission(a Authorizer, action Action, resourceFn ResourceFunc) func(http.Handler) http.Handler {
	return NewEnforcer(a).Require(action, resourceFn)
}

func writeError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "reason": reason})
}
