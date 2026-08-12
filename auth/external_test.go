package auth_test

import (
	"errors"
	"testing"

	"github.com/bsenel/karakuri/auth"
)

func TestExternalIdentityPrincipalID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prefix  string
		subject string
		want    string
		wantErr error
	}{
		{name: "namespaced", prefix: "oidc", subject: "8f3c-aa11", want: "oidc:8f3c-aa11"},
		{name: "saml prefix", prefix: "saml", subject: "alice@example.com", want: "saml:alice@example.com"},
		// The whole point of the prefix: a provider asserting the local
		// administrator's name must not resolve to the local administrator.
		{name: "cannot impersonate a local principal", prefix: "oidc", subject: "admin", want: "oidc:admin"},
		{name: "empty subject", prefix: "oidc", subject: "", wantErr: auth.ErrNoSubject},
		{name: "blank subject", prefix: "oidc", subject: "   ", wantErr: auth.ErrNoSubject},
		{name: "padded subject", prefix: "oidc", subject: " alice ", wantErr: auth.ErrInvalidSubject},
		{name: "control character", prefix: "oidc", subject: "alice\nadmin", wantErr: auth.ErrInvalidSubject},
		{name: "no prefix", prefix: "", subject: "alice", wantErr: auth.ErrNoPrefix},
		{name: "prefix with separator", prefix: "oi:dc", subject: "alice", wantErr: auth.ErrInvalidPattern},
		{name: "reserved prefix", prefix: "idp", subject: "alice", wantErr: auth.ErrReservedPrefix},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := auth.ExternalIdentity{Subject: tc.subject}.PrincipalID(tc.prefix)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("PrincipalID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExternalIdentityDisplayName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		identity auth.ExternalIdentity
		want     string
	}{
		{name: "name wins", identity: auth.ExternalIdentity{Subject: "s", Email: "a@b.c", Name: "Alice"}, want: "Alice"},
		{name: "email next", identity: auth.ExternalIdentity{Subject: "s", Email: "a@b.c"}, want: "a@b.c"},
		{name: "subject last", identity: auth.ExternalIdentity{Subject: "s"}, want: "s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.identity.DisplayName(); got != tc.want {
				t.Fatalf("DisplayName = %q, want %q", got, tc.want)
			}
		})
	}
}
