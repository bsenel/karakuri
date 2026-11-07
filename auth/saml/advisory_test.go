package saml_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/bsenel/karakuri/auth"
	karakurisaml "github.com/bsenel/karakuri/auth/saml"
	crewjam "github.com/crewjam/saml"
)

// These tests hold the guard for GO-2026-6909, which crewjam/saml v0.5.1 has
// no fix for. Each one drives the real identity provider into signing an
// assertion of the shape the advisory describes, so what is proven is that a
// genuinely signed assertion is refused, not that a hand-written one is.

// A signed assertion without a Subject makes crewjam dereference nil while
// validating it. That must end as an authentication failure the host hears
// about, not a panic that drops the connection.
func TestACSRecoversFromAssertionWithoutSubject(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	idp.Tamper = func(_ *testing.T, _ *stubIdP, req *crewjam.IdpAuthnRequest) {
		req.Assertion.Subject = nil
	}
	p, _, server := newProvider(t, idp, nil)
	mux := server.Config.Handler.(*http.ServeMux)

	_, resp, err := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if !errors.Is(err, karakurisaml.ErrMalformedAssertion) {
		t.Fatalf("err = %v, want ErrMalformedAssertion", err)
	}
}

// A NameID in a foreign namespace fills Subject.NameID as if it were the
// SAML one, so whoever can get such an element into a signed assertion picks
// the subject. It must be refused even when the signature is good.
func TestACSRejectsNameIDOutsideAssertionNamespace(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	idp.Tamper = func(t *testing.T, s *stubIdP, req *crewjam.IdpAuthnRequest) {
		el := req.Assertion.Element()
		subject := el.FindElement("./Subject")
		if subject == nil {
			t.Fatal("assertion has no Subject to tamper with")
		}
		subject.RemoveChild(subject.FindElement("./NameID"))
		spoof := etree.NewElement("x:NameID")
		spoof.CreateAttr("xmlns:x", "urn:example:not-saml")
		spoof.SetText("admin@example.com")
		subject.InsertChildAt(0, spoof)
		s.SignAssertion(t, req, el)
	}
	p, store, server := newProvider(t, idp, nil)
	mux := server.Config.Handler.(*http.ServeMux)

	principal, resp, err := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (principal %q got in)", resp.StatusCode, principal.ID)
	}
	if !errors.Is(err, karakurisaml.ErrForeignNameID) {
		t.Fatalf("err = %v, want ErrForeignNameID", err)
	}
	principals, listErr := store.ListPrincipals(t.Context())
	if listErr != nil {
		t.Fatalf("list principals: %v", listErr)
	}
	if len(principals) != 0 {
		t.Errorf("a refused assertion provisioned %d principal(s)", len(principals))
	}
}

// The guard reads the response before crewjam does, so it refuses what it
// cannot read: an artifact, which crewjam would fetch from the identity
// provider itself, and an encrypted assertion, whose NameID it cannot see.
func TestACSRefusesWhatTheGuardCannotRead(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	p, _, _ := newProvider(t, idp, nil)

	var lastErr error
	handler := p.ACSHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) {
			t.Error("onSuccess ran on a refused response")
		},
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			w.WriteHeader(http.StatusUnauthorized)
		},
	)

	encrypted := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">` +
		`<saml:EncryptedAssertion><EncryptedData/></saml:EncryptedAssertion></samlp:Response>`
	foreign := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">` +
		`<saml:Assertion><saml:Subject><NameID>admin@example.com</NameID></saml:Subject></saml:Assertion></samlp:Response>`

	cases := []struct {
		name string
		form url.Values
		want error
	}{
		{name: "artifact", form: url.Values{"SAMLart": {"AAQAAA"}}, want: karakurisaml.ErrArtifactBinding},
		{name: "encrypted assertion", form: url.Values{"SAMLResponse": {base64.StdEncoding.EncodeToString([]byte(encrypted))}}, want: karakurisaml.ErrEncryptedAssertion},
		{name: "unqualified NameID", form: url.Values{"SAMLResponse": {base64.StdEncoding.EncodeToString([]byte(foreign))}}, want: karakurisaml.ErrForeignNameID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastErr = nil
			r := httptest.NewRequest(http.MethodPost, "/saml/acs", strings.NewReader(tc.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if !errors.Is(lastErr, tc.want) {
				t.Fatalf("err = %v, want %v", lastErr, tc.want)
			}
		})
	}
}
