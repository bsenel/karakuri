package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
)

type flow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
}

func TestSealerRoundTrip(t *testing.T) {
	t.Parallel()
	s := auth.Sealer{Key: []byte("a-signing-key")}

	value, err := s.Seal(flow{State: "st", Nonce: "no", Verifier: "ve"}, time.Minute)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var got flow
	if err := s.Open(value, &got); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.State != "st" || got.Nonce != "no" || got.Verifier != "ve" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestSealerRejects(t *testing.T) {
	t.Parallel()
	s := auth.Sealer{Key: []byte("a-signing-key")}

	valid, err := s.Seal(flow{State: "st"}, time.Minute)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	payload, _, _ := strings.Cut(valid, ".")

	// A payload correctly signed but not valid JSON, so only the unmarshal can
	// catch it — and one that is not valid base64 either.
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	badBase64 := "!!!not-base64!!!"

	cases := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "no separator", value: "no-dot-here", wantErr: auth.ErrSealMalformed},
		{name: "signature is not base64", value: payload + ".!!!", wantErr: auth.ErrSealMalformed},
		{name: "signature is wrong", value: payload + "." + base64.RawURLEncoding.EncodeToString([]byte("wrong")), wantErr: auth.ErrSealSignature},
		{name: "envelope is not base64", value: signedBy(s, badBase64), wantErr: auth.ErrSealMalformed},
		{name: "envelope is not JSON", value: signedBy(s, notJSON), wantErr: auth.ErrSealMalformed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got flow
			if err := s.Open(tc.value, &got); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSealerExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := auth.Sealer{Key: []byte("k"), Now: func() time.Time { return now }}

	value, err := s.Seal(flow{State: "st"}, time.Minute)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	later := auth.Sealer{Key: []byte("k"), Now: func() time.Time { return now.Add(2 * time.Minute) }}
	var got flow
	if err := later.Open(value, &got); !errors.Is(err, auth.ErrSealExpired) {
		t.Fatalf("err = %v, want ErrSealExpired", err)
	}
}

// A value sealed with one key must not open with another, or a browser could
// mint its own state and the CSRF protection would be decorative.
func TestSealerRejectsForeignKey(t *testing.T) {
	t.Parallel()
	mine := auth.Sealer{Key: []byte("mine")}
	theirs := auth.Sealer{Key: []byte("theirs")}

	value, err := theirs.Seal(flow{State: "st"}, time.Minute)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var got flow
	if err := mine.Open(value, &got); !errors.Is(err, auth.ErrSealSignature) {
		t.Fatalf("err = %v, want ErrSealSignature", err)
	}
}

func TestSealerNeedsKey(t *testing.T) {
	t.Parallel()
	var s auth.Sealer

	if err := s.Validate(); !errors.Is(err, auth.ErrNoSealKey) {
		t.Fatalf("Validate = %v, want ErrNoSealKey", err)
	}
	if _, err := s.Seal(flow{}, time.Minute); !errors.Is(err, auth.ErrNoSealKey) {
		t.Fatalf("Seal = %v, want ErrNoSealKey", err)
	}
	var got flow
	if err := s.Open("anything.here", &got); !errors.Is(err, auth.ErrNoSealKey) {
		t.Fatalf("Open = %v, want ErrNoSealKey", err)
	}
	if err := (auth.Sealer{Key: []byte("k")}).Validate(); err != nil {
		t.Fatalf("Validate with a key = %v", err)
	}
}

func TestSealerUnsealableValue(t *testing.T) {
	t.Parallel()
	s := auth.Sealer{Key: []byte("k")}

	// A channel cannot be marshalled, so the error surfaces rather than being
	// swallowed into a cookie nobody can open.
	if _, err := s.Seal(make(chan int), time.Minute); err == nil {
		t.Fatal("Seal of an unmarshalable value returned nil")
	}
}

// signedBy reproduces the wire format around a deliberately broken envelope,
// so the signature check passes and only the decode can fail. It restates the
// format rather than reaching for an unexported helper — if the format ever
// changes, this test should be the thing that notices.
func signedBy(s auth.Sealer, encoded string) string {
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SECURITY_AUDIT.md F-16: SealEncrypted must round-trip, reject tampering, and —
// the point of it — must NOT leak the plaintext payload into the sealed string.
func TestSealEncryptedRoundTripAndConfidentiality(t *testing.T) {
	t.Parallel()
	s := auth.Sealer{Key: []byte("a-signing-and-encryption-key-32b!")}

	secret := "refresh-token-DEADBEEF-do-not-leak"
	value, err := s.SealEncrypted(flow{State: "st", Nonce: "no", Verifier: secret}, time.Minute)
	if err != nil {
		t.Fatalf("SealEncrypted: %v", err)
	}
	// The plaintext secret must not appear anywhere in the sealed string, nor
	// should a base64 decode of it reveal the JSON payload.
	if strings.Contains(value, secret) {
		t.Fatalf("sealed value leaks the plaintext secret")
	}
	if raw, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "\"v\"") {
			t.Fatalf("decoded sealed value leaks plaintext/JSON structure")
		}
	}

	var got flow
	if err := s.OpenEncrypted(value, &got); err != nil {
		t.Fatalf("OpenEncrypted: %v", err)
	}
	if got.Verifier != secret {
		t.Fatalf("round trip lost data: %+v", got)
	}

	// A signed-only Open must not accept an encrypted value, and vice versa.
	if err := s.Open(value, &got); err == nil {
		t.Fatalf("Open accepted an encrypted value")
	}

	// A different key cannot open it.
	other := auth.Sealer{Key: []byte("a-completely-different-key-here!!")}
	if err := other.OpenEncrypted(value, &got); err == nil {
		t.Fatalf("OpenEncrypted accepted a value sealed with a different key")
	}

	// Tampering with the ciphertext is rejected by the GCM tag.
	tampered := value[:len(value)-2] + "AA"
	if err := s.OpenEncrypted(tampered, &got); err == nil {
		t.Fatalf("OpenEncrypted accepted a tampered value")
	}
}
