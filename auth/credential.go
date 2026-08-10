package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"crypto/pbkdf2"
)

var (
	// ErrBadCredential is returned when a password does not verify, when a
	// principal has no password set, or when the stored hash is unreadable.
	// Callers must not distinguish these to the client — the failure modes are
	// deliberately indistinguishable from the outside.
	ErrBadCredential = errors.New("auth: invalid credentials")

	// ErrCredentialNotFound is returned by a store when a principal has no
	// credential record.
	ErrCredentialNotFound = errors.New("auth: credential not found")
)

// Credential is a principal's stored login material. Service principals hold no
// password — they authenticate with a rotating refresh token instead — so
// PasswordHash is empty for them.
type Credential struct {
	PrincipalID  string    `json:"principal_id"`
	PasswordHash string    `json:"-"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PasswordPolicy controls password hashing. Iterations is exposed so tests can
// run at a cost that does not dominate the suite; production uses the default.
type PasswordPolicy struct {
	Iterations int
}

// DefaultPasswordPolicy follows the OWASP recommendation for PBKDF2-HMAC-SHA256.
var DefaultPasswordPolicy = PasswordPolicy{Iterations: 600_000}

const (
	passwordScheme  = "pbkdf2-sha256"
	passwordSaltLen = 16
	passwordKeyLen  = 32
)

// Hash derives a storable password hash. The encoded form carries its own
// iteration count and salt, so raising Iterations later does not invalidate
// existing hashes — they keep verifying at the cost they were created with.
func (p PasswordPolicy) Hash(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("%w: password must not be empty", ErrBadCredential)
	}
	iterations := p.Iterations
	if iterations <= 0 {
		iterations = DefaultPasswordPolicy.Iterations
	}
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, passwordKeyLen)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		passwordScheme,
		strconv.Itoa(iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// HashPassword hashes with the default policy.
func HashPassword(password string) (string, error) { return DefaultPasswordPolicy.Hash(password) }

// VerifyPassword checks a password against an encoded hash in constant time.
// Every failure mode returns ErrBadCredential so callers cannot leak whether the
// principal exists, has a password, or simply typed it wrong.
func VerifyPassword(encoded, password string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return fmt.Errorf("%w: unrecognised password hash", ErrBadCredential)
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return fmt.Errorf("%w: unreadable iteration count", ErrBadCredential)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("%w: unreadable salt", ErrBadCredential)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return fmt.Errorf("%w: unreadable hash", ErrBadCredential)
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBadCredential, err)
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrBadCredential
	}
	return nil
}

// HashToken returns the SHA-256 hex digest of an opaque token. Refresh tokens
// are stored only as this digest: a leaked database yields no usable
// credential. Plain SHA-256 is right here (unlike for passwords) because the
// input is 256 bits of entropy from crypto/rand, not something guessable.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newOpaqueToken mints 256 bits of base64url-encoded randomness.
func newOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
