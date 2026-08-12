package auth

import (
	"crypto/sha256"
	"encoding/base64"
)

// CLIChallenge derives the value `krk auth login --sso` sends in place of its
// secret.
//
// It lives here rather than on either side because both compute it: the CLI
// when it starts a login, the server when it redeems the resulting code. Two
// implementations of "hash it" is two chances to disagree about the encoding,
// and the symptom of disagreeing would be that CLI login never works while
// every unit test on both sides passes.
func CLIChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
