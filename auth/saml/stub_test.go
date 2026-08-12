package saml_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	crewjam "github.com/crewjam/saml"
	"github.com/crewjam/saml/logger"
)

// stubIdP is a real crewjam/saml IdentityProvider, not a fake.
//
// SAML's whole surface is signed XML, so a hand-rolled response would prove
// only that this package can parse whatever the test happened to write. Driving
// the library's own IdP means signatures, audiences, destinations and time
// windows are all genuinely produced and genuinely verified.
type stubIdP struct {
	Server *httptest.Server
	IDP    *crewjam.IdentityProvider

	// Session is what the IdP asserts about the user. Tests mutate it.
	Session *crewjam.Session

	sps map[string]*crewjam.EntityDescriptor
}

func newStubIdP(t *testing.T) *stubIdP {
	t.Helper()
	key, cert := selfSigned(t, "idp.example.com")

	s := &stubIdP{
		sps: map[string]*crewjam.EntityDescriptor{},
		Session: &crewjam.Session{
			ID:           "session-1",
			CreateTime:   crewjam.TimeNow(),
			ExpireTime:   crewjam.TimeNow().Add(time.Hour),
			Index:        "index-1",
			NameID:       "alice@example.com",
			NameIDFormat: string(crewjam.EmailAddressNameIDFormat),
			UserName:     "alice",
			UserEmail:    "alice@example.com",
			Groups:       []string{"karakuri-operators"},
		},
	}

	mux := http.NewServeMux()
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)

	base := mustURL(t, s.Server.URL)
	s.IDP = &crewjam.IdentityProvider{
		Key:                     key,
		Certificate:             cert,
		Logger:                  logger.DefaultLogger,
		MetadataURL:             *base.JoinPath("/metadata"),
		SSOURL:                  *base.JoinPath("/sso"),
		ServiceProviderProvider: s,
		SessionProvider:         s,
	}

	mux.HandleFunc("/metadata", s.IDP.ServeMetadata)
	mux.HandleFunc("/sso", s.IDP.ServeSSO)
	return s
}

// Register makes a service provider known to the IdP, as an administrator
// would by uploading its metadata.
func (s *stubIdP) Register(sp *crewjam.ServiceProvider) {
	s.sps[sp.MetadataURL.String()] = sp.Metadata()
	s.sps[sp.EntityID] = sp.Metadata()
}

func (s *stubIdP) GetServiceProvider(_ *http.Request, id string) (*crewjam.EntityDescriptor, error) {
	if md, ok := s.sps[id]; ok {
		return md, nil
	}
	return nil, os.ErrNotExist
}

// GetSession stands in for the login page: the user is always authenticated.
func (s *stubIdP) GetSession(_ http.ResponseWriter, _ *http.Request, _ *crewjam.IdpAuthnRequest) *crewjam.Session {
	return s.Session
}

// Metadata returns the IdP's parsed metadata, which is what an SP is configured
// with.
func (s *stubIdP) Metadata() *crewjam.EntityDescriptor { return s.IDP.Metadata() }

func selfSigned(t *testing.T, cn string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return key, cert
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
