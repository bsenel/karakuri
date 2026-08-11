package llm

import (
	"context"
	"net/http"
	"strings"
)

// Customer attribution for a LiteLLM gateway.
//
// The gateway bills whichever customer a request names, and creates one on
// first use — so a twin needs nothing provisioned, only a header. The twin
// travels in the context rather than in CompletionRequest because every
// provider method already takes a context and none of them should have to know
// what a twin is.

type twinKey struct{}

// WithTwin marks a context as belonging to a twin, so model calls made under it
// are attributed to that twin's budget.
func WithTwin(ctx context.Context, twinID string) context.Context {
	if twinID == "" {
		return ctx
	}
	return context.WithValue(ctx, twinKey{}, twinID)
}

// TwinFromContext returns the twin a call belongs to, if any.
func TwinFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(twinKey{}).(string)
	return v, ok && v != ""
}

// CustomerHeader is the header LiteLLM resolves a customer from. It is first in
// the gateway's precedence order, ahead of x-litellm-end-user-id and the body's
// `user` field.
const CustomerHeader = "x-litellm-customer-id"

// customerPrefix namespaces the customer id so a twin cannot collide with
// whatever else shares the gateway. It matches quota.CustomerID, and the two
// disagreeing would mean spend attributed to nobody.
const customerPrefix = "twin:"

// customerTransport stamps the customer header on every outbound request.
//
// It exists because langchaingo's clients take an http.Client but offer no way
// to set a per-call header, and the twin varies per call — so the only place
// left to put it is the round trip.
type customerTransport struct {
	base   http.RoundTripper
	prefix string
}

// WithCustomerAttribution wraps an http.Client so requests carry the customer
// id of whichever twin the context names. Calls with no twin go unattributed
// rather than being charged to somebody arbitrary.
func WithCustomerAttribution(c *http.Client, prefix string) *http.Client {
	if c == nil {
		c = &http.Client{}
	}
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone := *c
	clone.Transport = &customerTransport{base: base, prefix: prefix}
	return &clone
}

func (t *customerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	twin, ok := TwinFromContext(r.Context())
	if !ok {
		return t.base.RoundTrip(r)
	}
	// Clone before mutating: a RoundTripper is not allowed to modify the
	// request it is given, and the caller may retry with the same one.
	clone := r.Clone(r.Context())
	clone.Header.Set(CustomerHeader, t.prefix+twin)
	return t.base.RoundTrip(clone)
}

// gatewayEnv returns env with the variables a CLI provider needs to route
// through a gateway and be attributed to the context's twin.
//
// A subprocess cannot be handed a per-call header any other way: the CLIs read
// their configuration from the environment, and Karakuri spawns them once per
// call, so the environment is where a per-twin value can live.
//
// Anything already set is left alone. An operator who has pointed the CLI
// somewhere deliberately should not have it overridden by this.
func gatewayEnv(ctx context.Context, env []string) []string {
	twin, ok := TwinFromContext(ctx)
	if !ok {
		return env
	}
	if hasEnv(env, "ANTHROPIC_CUSTOM_HEADERS") {
		return env
	}
	return append(env, "ANTHROPIC_CUSTOM_HEADERS="+CustomerHeader+": "+customerPrefix+twin)
}

func hasEnv(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
