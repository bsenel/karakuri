package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrSealMalformed is returned when a sealed value is not in the expected
	// shape at all.
	ErrSealMalformed = errors.New("auth: sealed value is malformed")

	// ErrSealSignature is returned when a sealed value's signature does not
	// verify — it was tampered with, or sealed by a different key.
	ErrSealSignature = errors.New("auth: sealed value signature is not valid")

	// ErrSealExpired is returned when a sealed value is past its lifetime.
	ErrSealExpired = errors.New("auth: sealed value has expired")

	// ErrNoSealKey is returned when a Sealer has no key.
	//
	// A generated fallback key would be worse than this error: it lives in one
	// process, so behind a load balancer a flow that starts on one replica and
	// returns to another fails intermittently, which is a bad thing to discover
	// in production.
	ErrNoSealKey = errors.New("auth: sealer needs a key")
)

// Sealer carries short-lived state through a browser and back.
//
// Federated login flows all have the same shape: redirect the browser to an
// identity provider, and validate what comes back against something only this
// application could have issued. That something has to survive a round trip
// through a user agent, and behind a load balancer it must survive landing on a
// different replica than the one that issued it — so it travels in a cookie
// rather than in memory.
//
// Values are signed, not encrypted. What a login flow carries — a state token,
// a nonce, a PKCE verifier, a SAML request ID — is not a secret from the
// browser holding it; the browser is the party it belongs to. What matters is
// that a browser cannot mint its own, and a MAC gives exactly that.
type Sealer struct {
	// Key signs and verifies. Required.
	Key []byte

	// Now overrides the clock. Intended for tests.
	Now func() time.Time
}

// Validate reports whether the sealer is usable. Call it at startup.
func (s Sealer) Validate() error {
	if len(s.Key) == 0 {
		return ErrNoSealKey
	}
	return nil
}

// sealed is the envelope. The payload is carried as raw JSON so a caller's own
// type round-trips through it untouched.
type sealed struct {
	Expires int64           `json:"e"`
	Data    json.RawMessage `json:"d"`
}

func (s Sealer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Seal encodes and signs v with a lifetime of ttl.
func (s Sealer) Seal(v any, ttl time.Duration) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	envelope, err := json.Marshal(sealed{Expires: s.now().Add(ttl).Unix(), Data: data})
	if err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(envelope)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(s.sign([]byte(encoded))), nil
}

// Open verifies a sealed value and decodes it into v.
//
// The signature is checked before anything is decoded: a value that did not
// come from this application is not worth parsing.
func (s Sealer) Open(value string, v any) error {
	if err := s.Validate(); err != nil {
		return err
	}
	encoded, signature, ok := strings.Cut(value, ".")
	if !ok {
		return ErrSealMalformed
	}
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return ErrSealMalformed
	}
	if !hmac.Equal(got, s.sign([]byte(encoded))) {
		return ErrSealSignature
	}

	envelope, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ErrSealMalformed
	}
	var out sealed
	if err := json.Unmarshal(envelope, &out); err != nil {
		return ErrSealMalformed
	}
	if s.now().Unix() > out.Expires {
		return ErrSealExpired
	}
	if err := json.Unmarshal(out.Data, v); err != nil {
		return ErrSealMalformed
	}
	return nil
}

func (s Sealer) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.Key)
	mac.Write(payload)
	return mac.Sum(nil)
}

// encryptLabel domain-separates the AES key derived below from the raw HMAC
// signing key, so the same Sealer.Key never serves two cryptographic purposes
// with the same bytes.
const encryptLabel = "karakuri/auth/seal/encrypt/v1"

// aeadKey derives a 32-byte AES-256 key from the sealer key.
func (s Sealer) aeadKey() [32]byte {
	h := sha256.New()
	h.Write(s.Key)
	h.Write([]byte(encryptLabel))
	var k [32]byte
	copy(k[:], h.Sum(nil))
	return k
}

func (s Sealer) aead() (cipher.AEAD, error) {
	key := s.aeadKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealEncrypted is Seal for a payload that must stay confidential from the party
// carrying it — a refresh token in a CLI handoff, for instance, which travels
// through a URL. Where Seal only signs (the browser may read what it holds),
// SealEncrypted additionally encrypts with AES-256-GCM, so the sealed string is
// ciphertext: it cannot be decoded into the underlying value without the key.
// GCM's tag also authenticates it, so no separate MAC is needed.
func (s Sealer) SealEncrypted(v any, ttl time.Duration) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	envelope, err := json.Marshal(sealed{Expires: s.now().Add(ttl).Unix(), Data: data})
	if err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	gcm, err := s.aead()
	if err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("auth: seal: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, envelope, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// OpenEncrypted verifies and decrypts a value produced by SealEncrypted.
func (s Sealer) OpenEncrypted(value string, v any) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return ErrSealMalformed
	}
	gcm, err := s.aead()
	if err != nil {
		return ErrSealMalformed
	}
	if len(raw) < gcm.NonceSize() {
		return ErrSealMalformed
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	envelope, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ErrSealSignature
	}
	var out sealed
	if err := json.Unmarshal(envelope, &out); err != nil {
		return ErrSealMalformed
	}
	if s.now().Unix() > out.Expires {
		return ErrSealExpired
	}
	if err := json.Unmarshal(out.Data, v); err != nil {
		return ErrSealMalformed
	}
	return nil
}
