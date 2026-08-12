package auth_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

func federationStore(t *testing.T) extauth.Store {
	t.Helper()
	store := extauth.NewMemoryStore()
	for _, role := range karakuriauth.BuiltinRoles() {
		if err := store.PutRole(context.Background(), role); err != nil {
			t.Fatalf("seed role %q: %v", role.Name, err)
		}
	}
	return store
}

func baseConfig() *config.Config {
	cfg := config.Default()
	cfg.Auth.JWT.Keys = []config.JWTKeyConfig{{
		ID: "test", Algorithm: "HS256", Active: true, Secret: "a-signing-secret",
	}}
	return cfg
}

func TestBuildFederationBearerIsInert(t *testing.T) {
	t.Parallel()
	cfg := baseConfig()

	f, err := karakuriauth.BuildFederation(context.Background(), cfg, federationStore(t))
	if err != nil {
		t.Fatalf("BuildFederation: %v", err)
	}
	if f.Enabled() {
		t.Error("bearer reported a federated provider")
	}
	if f.Resolver() != nil {
		t.Error("bearer contributed a resolver")
	}
	if f.Kind != config.AuthProviderBearer {
		t.Errorf("Kind = %q", f.Kind)
	}
}

func TestBuildFederationRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr error
		wantMsg string
	}{
		{
			name:    "unknown provider",
			mutate:  func(c *config.Config) { c.Auth.Provider = "ldap" },
			wantMsg: "not one of bearer, oidc, saml",
		},
		{
			// Deriving it from a request's Host header is how open redirects
			// start, so an operator has to say where the server lives.
			name: "no public URL",
			mutate: func(c *config.Config) {
				c.Auth.Provider = config.AuthProviderOIDC
				c.Auth.OIDC.IssuerURL = "https://idp.example.com"
			},
			wantErr: karakuriauth.ErrNoPublicURL,
		},
		{
			// A typo here would deny every login that matched the group, and it
			// belongs in startup output rather than somebody's browser.
			name: "role map names an unregistered role",
			mutate: func(c *config.Config) {
				c.Auth.Provider = config.AuthProviderOIDC
				c.Auth.Frontend.PublicURL = "https://karakuri.example.com"
				c.Auth.OIDC.IssuerURL = "https://idp.example.com"
				c.Auth.RoleMap.Groups = map[string][]string{"eng": {"wizrad"}}
			},
			wantErr: extauth.ErrRoleNotFound,
		},
		{
			name: "no signing material to derive a flow key from",
			mutate: func(c *config.Config) {
				c.Auth.Provider = config.AuthProviderOIDC
				c.Auth.Frontend.PublicURL = "https://karakuri.example.com"
				c.Auth.JWT.Keys = nil
			},
			wantErr: karakuriauth.ErrNoSigningKey,
		},
		{
			name: "saml with no metadata source",
			mutate: func(c *config.Config) {
				c.Auth.Provider = config.AuthProviderSAML
				c.Auth.Frontend.PublicURL = "https://karakuri.example.com"
			},
			wantMsg: "idp_metadata_url or idp_metadata_file",
		},
		{
			name: "saml metadata file is missing",
			mutate: func(c *config.Config) {
				c.Auth.Provider = config.AuthProviderSAML
				c.Auth.Frontend.PublicURL = "https://karakuri.example.com"
				c.Auth.SAML.IDPMetadataFile = filepath.Join(t.TempDir(), "absent.xml")
			},
			wantMsg: "read idp_metadata_file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseConfig()
			tc.mutate(cfg)

			_, err := karakuriauth.BuildFederation(context.Background(), cfg, federationStore(t))
			if err == nil {
				t.Fatal("BuildFederation returned nil")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantMsg)
			}
		})
	}
}

func TestBuildFederationSAMLFromMetadataFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "idp.xml")
	if err := os.WriteFile(path, []byte(idpMetadataXML), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	cfg := baseConfig()
	cfg.Auth.Provider = config.AuthProviderSAML
	cfg.Auth.Frontend.PublicURL = "https://karakuri.example.com/"
	cfg.Auth.SAML.IDPMetadataFile = path

	f, err := karakuriauth.BuildFederation(context.Background(), cfg, federationStore(t))
	if err != nil {
		t.Fatalf("BuildFederation: %v", err)
	}
	if !f.Enabled() {
		t.Fatal("SAML provider was not built")
	}
	// SAML contributes login handlers and no per-request resolver: an assertion
	// is a one-time login artifact, not a credential.
	if f.Resolver() != nil {
		t.Error("SAML contributed a resolver")
	}
	if got := f.SAML.ServiceProvider().AcsURL.String(); got != "https://karakuri.example.com/api/v1/auth/saml/acs" {
		t.Errorf("ACS URL = %q — the trailing slash on public_url was not trimmed", got)
	}
}

func TestBuildFederationRejectsMetadataWithoutIDP(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "idp.xml")
	body := `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com"></EntityDescriptor>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	cfg := baseConfig()
	cfg.Auth.Provider = config.AuthProviderSAML
	cfg.Auth.Frontend.PublicURL = "https://karakuri.example.com"
	cfg.Auth.SAML.IDPMetadataFile = path

	if _, err := karakuriauth.BuildFederation(context.Background(), cfg, federationStore(t)); err == nil {
		t.Fatal("metadata declaring no IDPSSODescriptor was accepted")
	}
}

// The post-login destination comes from configuration and never from the
// request: a "return to" parameter read off an SSO callback is an open
// redirect, and the callback is exactly where somebody would aim one.
func TestAbsoluteRedirect(t *testing.T) {
	t.Parallel()

	cases := []struct{ configured, want string }{
		{configured: "", want: "/"},
		{configured: "/dashboard", want: "/dashboard"},
		{configured: "dashboard", want: "/dashboard"},
		{configured: "https://app.example.com/home", want: "https://app.example.com/home"},
	}
	for _, tc := range cases {
		f := &karakuriauth.Federation{LoginRedirect: tc.configured}
		if got := f.AbsoluteRedirect(); got != tc.want {
			t.Errorf("AbsoluteRedirect(%q) = %q, want %q", tc.configured, got, tc.want)
		}
	}
}

// A nil Federation is what a zero-value AuthDeps carries, and the resolver
// chain asks it for a resolver on every request.
func TestFederationNilIsSafe(t *testing.T) {
	t.Parallel()
	var f *karakuriauth.Federation
	if f.Enabled() {
		t.Error("nil Federation reported enabled")
	}
	if f.Resolver() != nil {
		t.Error("nil Federation returned a resolver")
	}
}

// idpMetadataXML is the minimum a SAML identity provider has to publish for a
// service provider to be constructible against it.
const idpMetadataXML = `<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect"
                         Location="https://idp.example.com/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`
