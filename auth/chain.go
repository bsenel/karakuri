package auth

import (
	"errors"
	"net/http"
)

// ChainResolver tries several resolvers in order and takes the first principal
// one of them produces.
//
// It exists because a deployment that federates identity still has local
// credentials: the bootstrap administrator, service accounts, and — when the
// identity provider is unreachable — everybody. Chaining a JWTResolver ahead of
// a provider-backed one keeps local login working as the break-glass path
// without a second, long-lived static credential to protect.
//
// # Why a failed verification is not the end of the chain
//
// The obvious implementation stops at the first resolver that returns anything
// other than ErrNoCredential, on the reasoning that a malformed credential is a
// client bug worth surfacing. That is wrong here. A token minted by the identity
// provider *is* a bearer token in the Authorization header, so the local
// JWTResolver reaches it first and rejects it with a signature error — and the
// provider's resolver, which would have verified it happily, never runs.
//
// So the chain continues past verification failures and gives up only when
// every resolver has declined. The error it reports is the first one that was
// not simply "no credential here", so a genuinely bad token still explains
// itself rather than being reported as an absent one.
type ChainResolver []TokenResolver

var _ TokenResolver = (ChainResolver)(nil)

// Resolve returns the first principal any resolver in the chain produces.
func (c ChainResolver) Resolve(r *http.Request) (Principal, error) {
	var firstErr error
	for _, resolver := range c {
		if resolver == nil {
			continue
		}
		p, err := resolver.Resolve(r)
		if err == nil {
			return p, nil
		}
		if firstErr == nil && !errors.Is(err, ErrNoCredential) {
			firstErr = err
		}
	}
	if firstErr != nil {
		return Principal{}, firstErr
	}
	return Principal{}, ErrNoCredential
}
