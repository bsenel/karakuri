package jwt

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMalformedToken    = errors.New("jwt: malformed token")
	ErrAlgorithmMismatch = errors.New("jwt: token algorithm does not match the key")
	ErrSignatureInvalid  = errors.New("jwt: signature is not valid")
	ErrExpired           = errors.New("jwt: token has expired")
	ErrNotYetValid       = errors.New("jwt: token is not valid yet")
	ErrIssuerMismatch    = errors.New("jwt: unexpected issuer")
	ErrAudienceMismatch  = errors.New("jwt: unexpected audience")
	ErrTypeMismatch      = errors.New("jwt: unexpected token type")
	ErrMissingExpiry     = errors.New("jwt: token has no expiry")
	ErrCannotSign        = errors.New("jwt: key cannot sign")
)

// DefaultLeeway is the clock skew tolerated when checking exp/nbf.
const DefaultLeeway = 60 * time.Second

// Claims is the token payload. The registered claims follow RFC 7519; Roles,
// Scopes and Attrs are this project's private claims.
//
// Roles and Scopes are advisory — they let a UI render without an extra round
// trip. They are never the basis of an authorization decision: the authorizer
// re-reads bindings from the store on every request, so revoking a role takes
// effect immediately instead of whenever the access token happens to expire.
type Claims struct {
	Issuer    string `json:"iss,omitempty"`
	Subject   string `json:"sub,omitempty"`
	Audience  string `json:"aud,omitempty"`
	ExpiresAt int64  `json:"exp,omitempty"`
	NotBefore int64  `json:"nbf,omitempty"`
	IssuedAt  int64  `json:"iat,omitempty"`
	ID        string `json:"jti,omitempty"`

	Type   string            `json:"typ,omitempty"`
	Name   string            `json:"name,omitempty"`
	Kind   string            `json:"kind,omitempty"`
	Roles  []string          `json:"roles,omitempty"`
	Scopes []string          `json:"scopes,omitempty"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

// Expiry returns the exp claim as a time.
func (c Claims) Expiry() time.Time { return time.Unix(c.ExpiresAt, 0).UTC() }

type header struct {
	Alg Algorithm `json:"alg"`
	Typ string    `json:"typ"`
	Kid string    `json:"kid"`
}

// Validation describes what Parse must check beyond the signature. Zero values
// mean "do not check", except Leeway which falls back to DefaultLeeway and Now
// which falls back to time.Now.
type Validation struct {
	Issuer   string
	Audience string
	Type     string
	Leeway   time.Duration
	Now      func() time.Time
}

func (v Validation) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v Validation) leeway() time.Duration {
	if v.Leeway <= 0 {
		return DefaultLeeway
	}
	return v.Leeway
}

// Sign encodes and signs claims with the key, stamping its ID into the "kid"
// header so verifiers can pick the right key without trial decryption.
func Sign(c Claims, k Key) (string, error) {
	if !k.CanSign() {
		return "", fmt.Errorf("%w: %q", ErrCannotSign, k.ID)
	}
	headerJSON, err := json.Marshal(header{Alg: k.Alg, Typ: "JWT", Kid: k.ID})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signingInput := encodeSegment(headerJSON) + "." + encodeSegment(claimsJSON)
	sig, err := sign([]byte(signingInput), k)
	if err != nil {
		return "", err
	}
	return signingInput + "." + encodeSegment(sig), nil
}

// Parse verifies a compact JWS against the keyring and validates its claims.
//
// The key is selected by the token's "kid" and the header's "alg" must equal
// that key's algorithm — so a forged header can never steer verification onto a
// different primitive.
func Parse(token string, kr *Keyring, v Validation) (Claims, error) {
	var zero Claims
	if kr == nil {
		return zero, ErrNoActiveKey
	}

	rawHeader, rest, ok := strings.Cut(token, ".")
	if !ok {
		return zero, fmt.Errorf("%w: expected three segments", ErrMalformedToken)
	}
	rawClaims, rawSig, ok := strings.Cut(rest, ".")
	if !ok || strings.Contains(rawSig, ".") {
		return zero, fmt.Errorf("%w: expected three segments", ErrMalformedToken)
	}

	headerJSON, err := decodeSegment(rawHeader)
	if err != nil {
		return zero, fmt.Errorf("%w: header is not base64url", ErrMalformedToken)
	}
	var h header
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return zero, fmt.Errorf("%w: header is not JSON", ErrMalformedToken)
	}
	if h.Kid == "" {
		return zero, fmt.Errorf("%w: header has no kid", ErrMalformedToken)
	}
	// An unknown or absent algorithm — "none" included — never reaches a key.
	if h.Alg != HS256 && h.Alg != EdDSA {
		return zero, fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, h.Alg)
	}
	key, ok := kr.Get(h.Kid)
	if !ok {
		return zero, fmt.Errorf("%w: %q", ErrUnknownKey, h.Kid)
	}
	if h.Alg != key.Alg {
		return zero, fmt.Errorf("%w: token says %q, key %q is %q", ErrAlgorithmMismatch, h.Alg, key.ID, key.Alg)
	}

	sig, err := decodeSegment(rawSig)
	if err != nil {
		return zero, fmt.Errorf("%w: signature is not base64url", ErrMalformedToken)
	}
	if !verify([]byte(rawHeader+"."+rawClaims), sig, key) {
		return zero, ErrSignatureInvalid
	}

	claimsJSON, err := decodeSegment(rawClaims)
	if err != nil {
		return zero, fmt.Errorf("%w: payload is not base64url", ErrMalformedToken)
	}
	var c Claims
	if err := json.Unmarshal(claimsJSON, &c); err != nil {
		return zero, fmt.Errorf("%w: payload is not JSON", ErrMalformedToken)
	}
	if err := validate(c, v); err != nil {
		return zero, err
	}
	return c, nil
}

func validate(c Claims, v Validation) error {
	now, leeway := v.now(), v.leeway()

	// Every token this package issues expires. A token without an exp would
	// otherwise be valid forever, which defeats short-lived access tokens.
	if c.ExpiresAt == 0 {
		return ErrMissingExpiry
	}
	if now.Add(-leeway).After(time.Unix(c.ExpiresAt, 0)) {
		return fmt.Errorf("%w at %s", ErrExpired, time.Unix(c.ExpiresAt, 0).UTC().Format(time.RFC3339))
	}
	if c.NotBefore != 0 && now.Add(leeway).Before(time.Unix(c.NotBefore, 0)) {
		return fmt.Errorf("%w until %s", ErrNotYetValid, time.Unix(c.NotBefore, 0).UTC().Format(time.RFC3339))
	}
	if v.Issuer != "" && c.Issuer != v.Issuer {
		return fmt.Errorf("%w: got %q, want %q", ErrIssuerMismatch, c.Issuer, v.Issuer)
	}
	if v.Audience != "" && c.Audience != v.Audience {
		return fmt.Errorf("%w: got %q, want %q", ErrAudienceMismatch, c.Audience, v.Audience)
	}
	if v.Type != "" && c.Type != v.Type {
		return fmt.Errorf("%w: got %q, want %q", ErrTypeMismatch, c.Type, v.Type)
	}
	return nil
}

func sign(signingInput []byte, k Key) ([]byte, error) {
	switch k.Alg {
	case HS256:
		mac := hmac.New(sha256.New, k.Secret)
		mac.Write(signingInput)
		return mac.Sum(nil), nil
	case EdDSA:
		return ed25519.Sign(k.Private, signingInput), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, k.Alg)
	}
}

func verify(signingInput, sig []byte, k Key) bool {
	switch k.Alg {
	case HS256:
		mac := hmac.New(sha256.New, k.Secret)
		mac.Write(signingInput)
		return hmac.Equal(sig, mac.Sum(nil))
	case EdDSA:
		return ed25519.Verify(k.Public, signingInput, sig)
	default:
		return false
	}
}

func encodeSegment(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func decodeSegment(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
