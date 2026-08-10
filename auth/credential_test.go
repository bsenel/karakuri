package auth

import (
	"errors"
	"strings"
	"testing"
)

// testPolicy keeps PBKDF2 cheap so hashing does not dominate the suite. The
// encoded hash carries its own iteration count, so verification is unaffected.
var testPolicy = PasswordPolicy{Iterations: 2}

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := testPolicy.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$2$") {
		t.Fatalf("encoded form = %q", hash)
	}
	if err := VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Errorf("Verify(correct) = %v", err)
	}
	if err := VerifyPassword(hash, "wrong"); !errors.Is(err, ErrBadCredential) {
		t.Errorf("Verify(wrong) = %v, want ErrBadCredential", err)
	}

	// Salted: the same password hashes differently every time.
	other, _ := testPolicy.Hash("correct horse battery staple")
	if other == hash {
		t.Error("two hashes of the same password are identical — the salt is not random")
	}
}

func TestPasswordPolicyDefaults(t *testing.T) {
	// A non-positive iteration count falls back to the default rather than
	// silently hashing with zero rounds.
	hash, err := (PasswordPolicy{}).Hash("pw")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$600000$") {
		t.Fatalf("zero-iteration policy did not fall back to the default: %q", hash)
	}
	if err := VerifyPassword(hash, "pw"); err != nil {
		t.Errorf("Verify = %v", err)
	}

	if _, err := HashPassword(""); !errors.Is(err, ErrBadCredential) {
		t.Errorf("empty password = %v, want ErrBadCredential", err)
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	valid, _ := testPolicy.Hash("pw")
	parts := strings.Split(valid, "$")

	bad := map[string]string{
		"empty":              "",
		"wrong scheme":       "bcrypt$2$" + parts[2] + "$" + parts[3],
		"too few fields":     "pbkdf2-sha256$2$" + parts[2],
		"iterations not int": "pbkdf2-sha256$many$" + parts[2] + "$" + parts[3],
		"iterations zero":    "pbkdf2-sha256$0$" + parts[2] + "$" + parts[3],
		"salt not base64":    "pbkdf2-sha256$2$!!!$" + parts[3],
		"hash not base64":    "pbkdf2-sha256$2$" + parts[2] + "$!!!",
	}
	for name, encoded := range bad {
		if err := VerifyPassword(encoded, "pw"); !errors.Is(err, ErrBadCredential) {
			t.Errorf("%s = %v, want ErrBadCredential", name, err)
		}
	}
}

func TestHashToken(t *testing.T) {
	// Deterministic (it is a lookup key) and never the raw token.
	a, b := HashToken("secret"), HashToken("secret")
	if a != b {
		t.Error("HashToken is not deterministic")
	}
	if a == "secret" || len(a) != 64 {
		t.Errorf("HashToken = %q", a)
	}
	if HashToken("secret") == HashToken("secre7") {
		t.Error("distinct inputs collided")
	}
}

func TestNewOpaqueToken(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		tok, err := newOpaqueToken()
		if err != nil {
			t.Fatalf("newOpaqueToken: %v", err)
		}
		if len(tok) < 40 {
			t.Fatalf("token is only %d chars: %q", len(tok), tok)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}
