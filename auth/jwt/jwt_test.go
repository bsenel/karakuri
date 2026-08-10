package jwt

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func hmacKey(t *testing.T, id string) Key {
	t.Helper()
	k, err := NewHMACKey(id, []byte(strings.Repeat("s", 32)+id))
	if err != nil {
		t.Fatalf("NewHMACKey: %v", err)
	}
	return k
}

func ed25519Key(t *testing.T, id string) Key {
	t.Helper()
	k, err := GenerateEd25519Key(id)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	return k
}

func claims(exp time.Time) Claims {
	return Claims{
		Issuer:    "karakuri",
		Subject:   "alice",
		Audience:  "karakuri-api",
		ExpiresAt: exp.Unix(),
		IssuedAt:  time.Now().Unix(),
		Type:      "access",
		Roles:     []string{"operator"},
	}
}

func TestSignParseRoundTrip(t *testing.T) {
	for _, key := range []Key{hmacKey(t, "hs"), ed25519Key(t, "ed")} {
		t.Run(string(key.Alg), func(t *testing.T) {
			kr, err := NewKeyring(key)
			if err != nil {
				t.Fatalf("NewKeyring: %v", err)
			}
			token, err := Sign(claims(time.Now().Add(time.Hour)), key)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			got, err := Parse(token, kr, Validation{Issuer: "karakuri", Audience: "karakuri-api", Type: "access"})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.Subject != "alice" || len(got.Roles) != 1 || got.Roles[0] != "operator" {
				t.Fatalf("claims round-trip = %+v", got)
			}
			if !got.Expiry().After(time.Now()) {
				t.Errorf("Expiry() = %s", got.Expiry())
			}
		})
	}
}

func TestParseRejectsTampering(t *testing.T) {
	key := hmacKey(t, "hs")
	kr, _ := NewKeyring(key)
	token, _ := Sign(claims(time.Now().Add(time.Hour)), key)
	parts := strings.Split(token, ".")

	// Swap the payload for one claiming to be somebody else, keeping the
	// original signature.
	forged, _ := json.Marshal(Claims{Subject: "root", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]
	if _, err := Parse(tampered, kr, Validation{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Errorf("tampered payload = %v, want ErrSignatureInvalid", err)
	}

	// Flip a byte of the signature.
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[0] ^= 0xff
	bad := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)
	if _, err := Parse(bad, kr, Validation{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Errorf("flipped signature = %v, want ErrSignatureInvalid", err)
	}
}

func TestParseRejectsAlgNoneAndConfusion(t *testing.T) {
	hs := hmacKey(t, "hs")
	ed := ed25519Key(t, "ed")
	kr, _ := NewKeyring(hs, ed)

	// "alg": "none" with no signature at all — the classic bypass.
	none := forge(t, header{Alg: "none", Typ: "JWT", Kid: "hs"}, claims(time.Now().Add(time.Hour)), "")
	if _, err := Parse(none, kr, Validation{}); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Errorf("alg=none = %v, want ErrUnsupportedAlgorithm", err)
	}

	// Algorithm confusion: claim HS256 while pointing at the Ed25519 key, so a
	// naive verifier would HMAC with the public key as the secret.
	confused := forge(t, header{Alg: HS256, Typ: "JWT", Kid: "ed"}, claims(time.Now().Add(time.Hour)), "sig")
	if _, err := Parse(confused, kr, Validation{}); !errors.Is(err, ErrAlgorithmMismatch) {
		t.Errorf("alg confusion = %v, want ErrAlgorithmMismatch", err)
	}

	// Unknown algorithm.
	unknown := forge(t, header{Alg: "RS256", Typ: "JWT", Kid: "hs"}, claims(time.Now().Add(time.Hour)), "sig")
	if _, err := Parse(unknown, kr, Validation{}); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Errorf("alg=RS256 = %v, want ErrUnsupportedAlgorithm", err)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	kr, _ := NewKeyring(hmacKey(t, "hs"))
	valid, _ := Sign(claims(time.Now().Add(time.Hour)), hmacKey(t, "hs"))
	parts := strings.Split(valid, ".")

	cases := map[string]string{
		"empty":            "",
		"one segment":      "abc",
		"two segments":     "abc.def",
		"four segments":    valid + ".extra",
		"header not b64":   "!!!." + parts[1] + "." + parts[2],
		"header not json":  base64.RawURLEncoding.EncodeToString([]byte("nope")) + "." + parts[1] + "." + parts[2],
		"sig not b64":      parts[0] + "." + parts[1] + ".!!!",
		"no kid in header": forge(t, header{Alg: HS256, Typ: "JWT"}, claims(time.Now().Add(time.Hour)), "sig"),
	}
	for name, token := range cases {
		if _, err := Parse(token, kr, Validation{}); !errors.Is(err, ErrMalformedToken) {
			t.Errorf("%s = %v, want ErrMalformedToken", name, err)
		}
	}

	// A payload that survives signature verification but is not JSON.
	key := hmacKey(t, "hs")
	signingInput := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("not json"))
	sig, _ := sign([]byte(signingInput), key)
	badPayload := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
	if _, err := Parse(badPayload, kr, Validation{}); !errors.Is(err, ErrMalformedToken) {
		t.Errorf("non-JSON payload = %v, want ErrMalformedToken", err)
	}

	if _, err := Parse(valid, nil, Validation{}); !errors.Is(err, ErrNoActiveKey) {
		t.Errorf("nil keyring = %v, want ErrNoActiveKey", err)
	}
}

func TestParseUnknownKey(t *testing.T) {
	token, _ := Sign(claims(time.Now().Add(time.Hour)), hmacKey(t, "old"))
	kr, _ := NewKeyring(hmacKey(t, "new"))
	if _, err := Parse(token, kr, Validation{}); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown kid = %v, want ErrUnknownKey", err)
	}
}

func TestValidationClaims(t *testing.T) {
	key := hmacKey(t, "hs")
	kr, _ := NewKeyring(key)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	expired, _ := Sign(claims(now.Add(-2*time.Minute)), key)
	if _, err := Parse(expired, kr, Validation{Now: clock}); !errors.Is(err, ErrExpired) {
		t.Errorf("expired = %v, want ErrExpired", err)
	}
	// Inside the default 60s leeway it still passes — clocks drift.
	justExpired, _ := Sign(claims(now.Add(-30*time.Second)), key)
	if _, err := Parse(justExpired, kr, Validation{Now: clock}); err != nil {
		t.Errorf("within leeway = %v, want nil", err)
	}
	// …unless leeway is tightened.
	if _, err := Parse(justExpired, kr, Validation{Now: clock, Leeway: time.Second}); !errors.Is(err, ErrExpired) {
		t.Errorf("tight leeway = %v, want ErrExpired", err)
	}

	future := claims(now.Add(time.Hour))
	future.NotBefore = now.Add(10 * time.Minute).Unix()
	notYet, _ := Sign(future, key)
	if _, err := Parse(notYet, kr, Validation{Now: clock}); !errors.Is(err, ErrNotYetValid) {
		t.Errorf("nbf = %v, want ErrNotYetValid", err)
	}

	noExp, _ := Sign(Claims{Subject: "alice"}, key)
	if _, err := Parse(noExp, kr, Validation{Now: clock}); !errors.Is(err, ErrMissingExpiry) {
		t.Errorf("missing exp = %v, want ErrMissingExpiry", err)
	}

	good, _ := Sign(claims(now.Add(time.Hour)), key)
	if _, err := Parse(good, kr, Validation{Now: clock, Issuer: "other"}); !errors.Is(err, ErrIssuerMismatch) {
		t.Errorf("issuer = %v, want ErrIssuerMismatch", err)
	}
	if _, err := Parse(good, kr, Validation{Now: clock, Audience: "other"}); !errors.Is(err, ErrAudienceMismatch) {
		t.Errorf("audience = %v, want ErrAudienceMismatch", err)
	}
	if _, err := Parse(good, kr, Validation{Now: clock, Type: "refresh"}); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("type = %v, want ErrTypeMismatch", err)
	}
}

func TestKeyringRotation(t *testing.T) {
	oldKey, newKey := hmacKey(t, "k1"), hmacKey(t, "k2")
	kr, err := NewKeyring(oldKey)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	issuedUnderOld, _ := Sign(claims(time.Now().Add(time.Hour)), oldKey)

	// Rotate: add the new key and make it active. Tokens already in flight keep
	// verifying against the old key until they expire on their own.
	if err := kr.Add(newKey); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := kr.SetActive("k2"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	active, err := kr.Active()
	if err != nil || active.ID != "k2" {
		t.Fatalf("Active = %+v, %v", active, err)
	}
	if _, err := Parse(issuedUnderOld, kr, Validation{}); err != nil {
		t.Errorf("token issued under the old key = %v, want it to still verify", err)
	}
	issuedUnderNew, _ := Sign(claims(time.Now().Add(time.Hour)), active)
	if _, err := Parse(issuedUnderNew, kr, Validation{}); err != nil {
		t.Errorf("token issued under the new key = %v", err)
	}
	if len(kr.IDs()) != 2 {
		t.Errorf("IDs() = %v", kr.IDs())
	}
	if _, ok := kr.Get("nope"); ok {
		t.Error("Get returned ok for an unknown key")
	}
}

func TestKeyConstructors(t *testing.T) {
	if _, err := NewHMACKey("", []byte(strings.Repeat("s", 32))); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("empty id = %v", err)
	}
	// A secret weaker than the digest is a real weakness, not a style choice.
	if _, err := NewHMACKey("k", []byte("short")); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("short secret = %v, want ErrInvalidKey", err)
	}
	if _, err := NewEd25519Key("k", ed25519.PrivateKey("too short")); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("bad private key = %v", err)
	}
	if _, err := NewEd25519Key("", ed25519Key(t, "x").Private); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("empty id = %v", err)
	}
	if _, err := GenerateEd25519Key(""); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("generate with empty id = %v", err)
	}

	pub := ed25519Key(t, "x").Public
	verifyOnly, err := NewEd25519VerifyKey("v", pub)
	if err != nil {
		t.Fatalf("NewEd25519VerifyKey: %v", err)
	}
	if verifyOnly.CanSign() {
		t.Error("a verify-only key reports CanSign")
	}
	if _, err := Sign(claims(time.Now().Add(time.Hour)), verifyOnly); !errors.Is(err, ErrCannotSign) {
		t.Errorf("signing with a verify-only key = %v, want ErrCannotSign", err)
	}
	if _, err := NewEd25519VerifyKey("", pub); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("empty id = %v", err)
	}
	if _, err := NewEd25519VerifyKey("v", ed25519.PublicKey("short")); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("short public key = %v", err)
	}
	if (Key{Alg: "RS256"}).CanSign() {
		t.Error("an unsupported algorithm reports CanSign")
	}
}

func TestKeyringConstruction(t *testing.T) {
	if _, err := NewKeyring(); !errors.Is(err, ErrNoActiveKey) {
		t.Errorf("empty keyring = %v", err)
	}
	if _, err := NewKeyring(Key{ID: "k", Alg: "RS256"}); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Errorf("unsupported alg = %v", err)
	}
	if _, err := NewKeyring(Key{Alg: HS256, Secret: []byte(strings.Repeat("s", 32))}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("missing id = %v", err)
	}
	if _, err := NewKeyring(Key{ID: "k", Alg: HS256, Secret: []byte("short")}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("short secret = %v", err)
	}
	if _, err := NewKeyring(Key{ID: "k", Alg: EdDSA}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("EdDSA without a public half = %v", err)
	}
	// A verify-only key cannot be the initial (and therefore active) key.
	verifyOnly, _ := NewEd25519VerifyKey("v", ed25519Key(t, "x").Public)
	if _, err := NewKeyring(verifyOnly); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("verify-only active key = %v, want ErrInvalidKey", err)
	}

	kr, _ := NewKeyring(hmacKey(t, "k1"))
	if err := kr.Add(verifyOnly); err != nil {
		t.Fatalf("Add(verify-only): %v", err)
	}
	if err := kr.SetActive("v"); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("SetActive(verify-only) = %v, want ErrInvalidKey", err)
	}
	if err := kr.SetActive("nope"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("SetActive(unknown) = %v", err)
	}
	if err := kr.Add(Key{Alg: HS256, Secret: []byte(strings.Repeat("s", 32))}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Add without id = %v", err)
	}

	// A keyring whose active key vanished reports it rather than signing with
	// an arbitrary one.
	broken := &Keyring{active: "gone", keys: map[string]Key{}}
	if _, err := broken.Active(); !errors.Is(err, ErrNoActiveKey) {
		t.Errorf("Active on a broken ring = %v", err)
	}
}

func TestSignUnsupportedAlgorithm(t *testing.T) {
	// CanSign gates the public path, so reach the switch default directly.
	if _, err := sign([]byte("x"), Key{ID: "k", Alg: "RS256"}); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Errorf("sign with RS256 = %v", err)
	}
	if verify([]byte("x"), []byte("y"), Key{ID: "k", Alg: "RS256"}) {
		t.Error("verify accepted an unsupported algorithm")
	}
}

// forge builds a token with an arbitrary header, for negative tests.
func forge(t *testing.T, h header, c Claims, sig string) string {
	t.Helper()
	hj, _ := json.Marshal(h)
	cj, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(hj) + "." +
		base64.RawURLEncoding.EncodeToString(cj) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(sig))
}
