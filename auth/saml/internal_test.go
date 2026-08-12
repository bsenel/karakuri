package saml

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/auth"
	crewjam "github.com/crewjam/saml"
)

// assertion builds a verified-looking assertion. Reading attributes out of one
// is pure, so it is worth testing directly rather than only through a signed
// round trip — the round trip proves the plumbing, these prove the mapping.
func assertion(nameID string, statements ...crewjam.AttributeStatement) *crewjam.Assertion {
	a := &crewjam.Assertion{
		Issuer:              crewjam.Issuer{Value: "https://idp.example.com"},
		AttributeStatements: statements,
	}
	if nameID != "" {
		a.Subject = &crewjam.Subject{NameID: &crewjam.NameID{Value: nameID}}
	}
	return a
}

func statement(attrs ...crewjam.Attribute) crewjam.AttributeStatement {
	return crewjam.AttributeStatement{Attributes: attrs}
}

func attr(name, friendly string, values ...string) crewjam.Attribute {
	a := crewjam.Attribute{Name: name, FriendlyName: friendly}
	for _, v := range values {
		a.Values = append(a.Values, crewjam.AttributeValue{Value: v})
	}
	return a
}

func testProvider(cfg Config) *Provider {
	return &Provider{cfg: cfg.withDefaults(), sealer: auth.Sealer{Key: []byte("k")}}
}

func TestIdentityFromAssertion(t *testing.T) {
	t.Parallel()
	p := testProvider(Config{RoleAttribute: "groups", EmailAttribute: "mail", NameAttribute: "displayName"})

	got, err := p.identityFromAssertion(assertion("alice", statement(
		attr("groups", "", "eng", "ops"),
		attr("mail", "", "alice@example.com"),
		attr("displayName", "", "Alice"),
	)))
	if err != nil {
		t.Fatalf("identityFromAssertion: %v", err)
	}
	if got.Subject != "alice" || got.Email != "alice@example.com" || got.Name != "Alice" {
		t.Fatalf("identity = %+v", got)
	}
	if !slices.Equal(got.Groups, []string{"eng", "ops"}) {
		t.Fatalf("Groups = %v", got.Groups)
	}
	if got.Issuer != "https://idp.example.com" {
		t.Errorf("Issuer = %q", got.Issuer)
	}
}

// A NameID is very often the email address already, and a principal listing
// reading "saml:alice@example.com" with no email is needlessly worse.
func TestIdentityEmailFallsBackToNameID(t *testing.T) {
	t.Parallel()
	p := testProvider(Config{})

	got, err := p.identityFromAssertion(assertion("alice@example.com"))
	if err != nil {
		t.Fatalf("identityFromAssertion: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Fatalf("Email = %q, want the NameID", got.Email)
	}

	// A NameID that is not an email must not be pressed into service as one.
	opaque, err := p.identityFromAssertion(assertion("S-1-5-21-1004336348"))
	if err != nil {
		t.Fatalf("identityFromAssertion: %v", err)
	}
	if opaque.Email != "" {
		t.Fatalf("Email = %q, want empty for an opaque NameID", opaque.Email)
	}
}

func TestIdentityNeedsSubject(t *testing.T) {
	t.Parallel()
	p := testProvider(Config{})

	for _, a := range []*crewjam.Assertion{
		assertion(""),
		{Subject: &crewjam.Subject{}},
		{Subject: &crewjam.Subject{NameID: &crewjam.NameID{Value: ""}}},
	} {
		if _, err := p.identityFromAssertion(a); !errors.Is(err, ErrNoSubject) {
			t.Fatalf("err = %v, want ErrNoSubject", err)
		}
	}
}

func TestAttributeValues(t *testing.T) {
	t.Parallel()

	a := assertion("alice",
		statement(attr("urn:oid:1.3.6.1.4.1.5923.1.1.1.1", "eduPersonAffiliation", "staff", " padded ", "", "staff")),
		statement(attr("http://schemas.xmlsoap.org/claims/Group", "", "Domain Users")),
	)

	cases := []struct {
		name string
		look string
		want []string
	}{
		{name: "by Name", look: "urn:oid:1.3.6.1.4.1.5923.1.1.1.1", want: []string{"staff", "padded"}},
		{name: "by FriendlyName", look: "eduPersonAffiliation", want: []string{"staff", "padded"}},
		{name: "ADFS-style URI", look: "http://schemas.xmlsoap.org/claims/Group", want: []string{"Domain Users"}},
		{name: "unknown attribute", look: "nope", want: nil},
		{name: "empty configuration", look: "", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := attributeValues(a, tc.look); !slices.Equal(got, tc.want) {
				t.Fatalf("attributeValues(%q) = %v, want %v", tc.look, got, tc.want)
			}
		})
	}
}

// Correlating the response to a request we sent is the default. Turning it off
// is a documented trade, not an accident.
func TestPossibleRequestIDs(t *testing.T) {
	t.Parallel()

	p := testProvider(Config{})
	sealed, err := p.sealer.Seal(flowState{RequestID: "id-42"}, p.cfg.StateTTL)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	withCookie := func(value string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/saml/acs", nil)
		if value != "" {
			r.AddCookie(&http.Cookie{Name: p.cfg.CookieName, Value: value})
		}
		return r
	}

	if got := p.possibleRequestIDs(withCookie(sealed)); !slices.Equal(got, []string{"id-42"}) {
		t.Errorf("with a valid cookie = %v, want [id-42]", got)
	}
	if got := p.possibleRequestIDs(withCookie("")); got != nil {
		t.Errorf("with no cookie = %v, want nothing — an unsolicited response is refused", got)
	}
	if got := p.possibleRequestIDs(withCookie("forged.value")); got != nil {
		t.Errorf("with a forged cookie = %v, want nothing", got)
	}

	idpInitiated := testProvider(Config{AllowIDPInitiated: true})
	if got := idpInitiated.possibleRequestIDs(withCookie("")); !slices.Equal(got, []string{""}) {
		t.Errorf("IdP-initiated = %v, want the empty ID crewjam reads as unsolicited", got)
	}
}

// A login that cannot be sealed must not redirect the user to an identity
// provider they will never get back from.
func TestLoginHandlerFailsWithoutSealKey(t *testing.T) {
	t.Parallel()
	idpMetadata := &crewjam.EntityDescriptor{
		EntityID: "https://idp.example.com",
		IDPSSODescriptors: []crewjam.IDPSSODescriptor{{
			SSODescriptor: crewjam.SSODescriptor{},
			SingleSignOnServices: []crewjam.Endpoint{{
				Binding:  crewjam.HTTPRedirectBinding,
				Location: "https://idp.example.com/sso",
			}},
		}},
	}
	p := &Provider{
		cfg: Config{IDPMetadata: idpMetadata, ACSURL: "https://sp.example.com/acs"}.withDefaults(),
		sp: crewjam.ServiceProvider{
			EntityID:    "https://sp.example.com/metadata",
			IDPMetadata: idpMetadata,
		},
		sealer: auth.Sealer{}, // no key
	}

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/login", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a failed login still set a flow cookie")
	}
}

// An identity provider whose metadata advertises no redirect endpoint cannot be
// reached, and saying so beats redirecting to the empty string.
func TestLoginHandlerFailsWithoutSSOEndpoint(t *testing.T) {
	t.Parallel()
	p := &Provider{
		cfg:    Config{}.withDefaults(),
		sp:     crewjam.ServiceProvider{IDPMetadata: &crewjam.EntityDescriptor{}},
		sealer: auth.Sealer{Key: []byte("k")},
	}

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/login", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
