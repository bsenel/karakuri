package auth

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/bsenel/karakuri/auth/jwt"
	"github.com/bsenel/karakuri/config"
)

// ErrNoSigningKey is returned when the configuration declares no usable signing
// key. It is fatal at startup by design: falling back to a generated or
// well-known default would mean tokens minted by one process verify in another,
// or that anyone reading the source can mint their own.
var ErrNoSigningKey = errors.New("auth: no JWT signing key configured")

// NewKeyring builds the verification keyring from configuration. Exactly one
// key signs; the rest keep previously-issued tokens verifiable, so rotating the
// signer does not invalidate everything already in flight.
func NewKeyring(cfg config.JWTConfig) (*jwt.Keyring, error) {
	if len(cfg.Keys) == 0 {
		return nil, fmt.Errorf("%w: set KARAKURI_AUTH_JWT_SECRET or declare auth.jwt.keys", ErrNoSigningKey)
	}

	keys := make([]jwt.Key, 0, len(cfg.Keys))
	activeIdx := -1
	for i, kc := range cfg.Keys {
		key, err := buildKey(kc)
		if err != nil {
			return nil, fmt.Errorf("auth.jwt.keys[%d] (kid %q): %w", i, kc.ID, err)
		}
		if kc.Active && key.CanSign() {
			activeIdx = len(keys)
		}
		keys = append(keys, key)
	}

	// NewKeyring signs with the first key, so put the active one there.
	if activeIdx > 0 {
		keys[0], keys[activeIdx] = keys[activeIdx], keys[0]
	}
	if activeIdx < 0 {
		// No key was flagged active; the first signable key is the signer.
		signable := -1
		for i, k := range keys {
			if k.CanSign() {
				signable = i
				break
			}
		}
		if signable < 0 {
			return nil, fmt.Errorf("%w: every configured key is verify-only", ErrNoSigningKey)
		}
		keys[0], keys[signable] = keys[signable], keys[0]
	}

	return jwt.NewKeyring(keys...)
}

func buildKey(kc config.JWTKeyConfig) (jwt.Key, error) {
	id := kc.ID
	if id == "" {
		id = "default"
	}
	switch kc.Algorithm {
	case "", "HS256":
		if kc.Secret == "" {
			return jwt.Key{}, errors.New("HS256 key has no secret (set `secret_env` or KARAKURI_AUTH_JWT_SECRET)")
		}
		return jwt.NewHMACKey(id, []byte(kc.Secret))
	case "EdDSA":
		if kc.PrivateKeyFile != "" {
			priv, err := readEd25519PrivateKey(kc.PrivateKeyFile)
			if err != nil {
				return jwt.Key{}, err
			}
			return jwt.NewEd25519Key(id, priv)
		}
		if kc.PublicKeyFile != "" {
			pub, err := readEd25519PublicKey(kc.PublicKeyFile)
			if err != nil {
				return jwt.Key{}, err
			}
			return jwt.NewEd25519VerifyKey(id, pub)
		}
		return jwt.Key{}, errors.New("EdDSA key needs private_key_file or public_key_file")
	default:
		return jwt.Key{}, fmt.Errorf("unsupported algorithm %q (want HS256 or EdDSA)", kc.Algorithm)
	}
}

func readEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", path, err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an ed25519 private key", path)
	}
	return priv, nil
}

func readEd25519PublicKey(path string) (ed25519.PublicKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key %s: %w", path, err)
	}
	pub, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an ed25519 public key", path)
	}
	return pub, nil
}

func readPEM(path string) (*pem.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s is not PEM-encoded", path)
	}
	return block, nil
}
