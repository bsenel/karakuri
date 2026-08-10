// Package jwt implements the subset of RFC 7519 this project needs — signing
// and verifying compact JWS tokens — over the standard library alone.
//
// Only two algorithms exist here, and both are chosen from a fixed allowlist at
// verification time. The classic JWT vulnerabilities are closed by construction:
//
//   - "alg": "none" is rejected, because an unknown algorithm never resolves to
//     a key.
//   - Algorithm confusion is impossible: a token's "kid" selects the key first,
//     and the header's "alg" must equal that key's own algorithm. An attacker
//     cannot present an HS256 token signed with an Ed25519 public key.
//   - Signature comparison is constant-time.
package jwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
)

// Algorithm is a JWS "alg" header value. The set is closed.
type Algorithm string

const (
	// HS256 is HMAC-SHA256 with a shared secret. Simplest to operate: one
	// secret, no key distribution.
	HS256 Algorithm = "HS256"

	// EdDSA is Ed25519. Use when verifiers should not be able to mint tokens —
	// they hold only the public half.
	EdDSA Algorithm = "EdDSA"
)

var (
	ErrUnsupportedAlgorithm = errors.New("jwt: unsupported algorithm")
	ErrInvalidKey           = errors.New("jwt: invalid key")
	ErrUnknownKey           = errors.New("jwt: unknown key id")
	ErrNoActiveKey          = errors.New("jwt: keyring has no active key")
)

// Key is one signing/verification key in a keyring. Exactly one of Secret or
// the Ed25519 pair is populated, according to Alg.
type Key struct {
	ID  string
	Alg Algorithm

	// Secret backs HS256.
	Secret []byte

	// Private may be nil on a verify-only Ed25519 key.
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

// NewHMACKey builds an HS256 key. Secrets shorter than 32 bytes are rejected:
// an HMAC key weaker than its digest is a real weakness, not a style preference.
func NewHMACKey(id string, secret []byte) (Key, error) {
	if id == "" {
		return Key{}, fmt.Errorf("%w: key id is required", ErrInvalidKey)
	}
	if len(secret) < 32 {
		return Key{}, fmt.Errorf("%w: HS256 secret must be at least 32 bytes, got %d", ErrInvalidKey, len(secret))
	}
	return Key{ID: id, Alg: HS256, Secret: secret}, nil
}

// NewEd25519Key builds an EdDSA key from a private key.
func NewEd25519Key(id string, priv ed25519.PrivateKey) (Key, error) {
	if id == "" {
		return Key{}, fmt.Errorf("%w: key id is required", ErrInvalidKey)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return Key{}, fmt.Errorf("%w: ed25519 private key must be %d bytes, got %d", ErrInvalidKey, ed25519.PrivateKeySize, len(priv))
	}
	// Safe: ed25519.PrivateKey.Public always returns an ed25519.PublicKey, and
	// the length check above rules out the zero value.
	pub := priv.Public().(ed25519.PublicKey)
	return Key{ID: id, Alg: EdDSA, Private: priv, Public: pub}, nil
}

// NewEd25519VerifyKey builds a verify-only EdDSA key. Signing with it fails.
func NewEd25519VerifyKey(id string, pub ed25519.PublicKey) (Key, error) {
	if id == "" {
		return Key{}, fmt.Errorf("%w: key id is required", ErrInvalidKey)
	}
	if len(pub) != ed25519.PublicKeySize {
		return Key{}, fmt.Errorf("%w: ed25519 public key must be %d bytes, got %d", ErrInvalidKey, ed25519.PublicKeySize, len(pub))
	}
	return Key{ID: id, Alg: EdDSA, Public: pub}, nil
}

// GenerateEd25519Key mints a fresh EdDSA keypair.
func GenerateEd25519Key(id string) (Key, error) {
	if id == "" {
		return Key{}, fmt.Errorf("%w: key id is required", ErrInvalidKey)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: id, Alg: EdDSA, Private: priv, Public: pub}, nil
}

// CanSign reports whether the key carries signing material.
func (k Key) CanSign() bool {
	switch k.Alg {
	case HS256:
		return len(k.Secret) > 0
	case EdDSA:
		return len(k.Private) == ed25519.PrivateKeySize
	default:
		return false
	}
}

// Keyring holds every key a verifier will accept plus the one signer currently
// in use. Rotation is therefore non-disruptive: add the new key, make it active,
// and tokens already in flight keep verifying against the old one until they
// expire on their own.
type Keyring struct {
	active string
	keys   map[string]Key
}

// NewKeyring builds a keyring. The first key is active unless SetActive says
// otherwise.
func NewKeyring(keys ...Key) (*Keyring, error) {
	if len(keys) == 0 {
		return nil, ErrNoActiveKey
	}
	kr := &Keyring{keys: make(map[string]Key, len(keys))}
	for _, k := range keys {
		if err := kr.Add(k); err != nil {
			return nil, err
		}
	}
	kr.active = keys[0].ID
	if !kr.keys[kr.active].CanSign() {
		return nil, fmt.Errorf("%w: first key %q cannot sign", ErrInvalidKey, kr.active)
	}
	return kr, nil
}

// Add registers a key for verification.
func (kr *Keyring) Add(k Key) error {
	if k.ID == "" {
		return fmt.Errorf("%w: key id is required", ErrInvalidKey)
	}
	if k.Alg != HS256 && k.Alg != EdDSA {
		return fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, k.Alg)
	}
	if k.Alg == HS256 && len(k.Secret) < 32 {
		return fmt.Errorf("%w: HS256 secret for %q must be at least 32 bytes", ErrInvalidKey, k.ID)
	}
	if k.Alg == EdDSA && len(k.Public) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: EdDSA key %q has no usable public half", ErrInvalidKey, k.ID)
	}
	kr.keys[k.ID] = k
	return nil
}

// SetActive selects the signing key. The key must already be in the ring and
// must carry signing material.
func (kr *Keyring) SetActive(id string) error {
	k, ok := kr.keys[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKey, id)
	}
	if !k.CanSign() {
		return fmt.Errorf("%w: key %q cannot sign", ErrInvalidKey, id)
	}
	kr.active = id
	return nil
}

// Active returns the current signing key.
func (kr *Keyring) Active() (Key, error) {
	k, ok := kr.keys[kr.active]
	if !ok {
		return Key{}, ErrNoActiveKey
	}
	return k, nil
}

// Get returns a key by ID.
func (kr *Keyring) Get(id string) (Key, bool) {
	k, ok := kr.keys[id]
	return k, ok
}

// IDs returns every key ID in the ring.
func (kr *Keyring) IDs() []string {
	out := make([]string, 0, len(kr.keys))
	for id := range kr.keys {
		out = append(out, id)
	}
	return out
}
