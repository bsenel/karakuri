package oidc

import (
	"crypto/rand"
	"encoding/base64"
)

// randomString returns n bytes of cryptographic randomness, URL-safe.
//
// State and nonce are both anti-replay values, so their only requirement is
// that an attacker cannot predict one. crypto/rand is the only acceptable
// source, and its error is returned rather than ignored: a login that proceeds
// with a predictable state is worse than a login that fails.
func randomString(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
