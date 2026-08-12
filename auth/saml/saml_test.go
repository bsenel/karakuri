package saml_test

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	karakurisaml "github.com/bsenel/karakuri/auth/saml"
	crewjam "github.com/crewjam/saml"
)

func newProvisioner(t *testing.T) (*auth.Provisioner, auth.Store) {
	t.Helper()
	store := auth.NewMemoryStore()
	for _, name := range []string{"admin", "operator", "viewer"} {
		if err := store.PutRole(context.Background(), auth.Role{Name: name}); err != nil {
			t.Fatalf("seed role: %v", err)
		}
	}
	return &auth.Provisioner{
		Store:  store,
		Prefix: karakurisaml.DefaultPrefix,
		Roles: auth.RoleMap{Groups: map[string][]auth.RoleGrant{
			"karakuri-operators": {{Role: "operator"}},
			"karakuri-admins":    {{Role: "admin"}},
		}},
	}, store
}

// sp wires a Provider to a live test server and registers it with the IdP, the
// way an administrator registers a real one.
func newProvider(t *testing.T, idp *stubIdP, mutate func(*karakurisaml.Config)) (*karakurisaml.Provider, auth.Store, *httptest.Server) {
	t.Helper()
	provisioner, store := newProvisioner(t)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cfg := karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       idp.Metadata(),
		RoleAttribute:     "eduPersonAffiliation",
		EmailAttribute:    "mail",
		NameAttribute:     "uid",
		StateKey:          []byte("a-state-signing-key"),
		InsecureAllowHTTP: true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	p, err := karakurisaml.New(cfg, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idp.Register(p.ServiceProvider())

	mux.Handle("/saml/metadata", p.MetadataHandler())
	mux.Handle("/saml/login", p.LoginHandler())
	return p, store, server
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	provisioner, _ := newProvisioner(t)
	good := karakurisaml.Config{
		IDPMetadata: idp.Metadata(),
		ACSURL:      "https://karakuri.example.com/saml/acs",
		StateKey:    []byte("k"),
	}

	cases := []struct {
		name    string
		mutate  func(*karakurisaml.Config)
		wantErr error
	}{
		{name: "no IdP metadata", mutate: func(c *karakurisaml.Config) { c.IDPMetadata = nil }, wantErr: karakurisaml.ErrNoIDPMetadata},
		{name: "no ACS URL", mutate: func(c *karakurisaml.Config) { c.ACSURL = "" }, wantErr: karakurisaml.ErrNoACSURL},
		{name: "blank ACS URL", mutate: func(c *karakurisaml.Config) { c.ACSURL = "  " }, wantErr: karakurisaml.ErrNoACSURL},
		{name: "no state key", mutate: func(c *karakurisaml.Config) { c.StateKey = nil }, wantErr: karakurisaml.ErrNoStateKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := good
			tc.mutate(&cfg)
			if _, err := karakurisaml.New(cfg, provisioner); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("no provisioner", func(t *testing.T) {
		t.Parallel()
		if _, err := karakurisaml.New(good, nil); !errors.Is(err, karakurisaml.ErrNoProvisioner) {
			t.Fatalf("err = %v, want ErrNoProvisioner", err)
		}
	})

	t.Run("unparseable URLs", func(t *testing.T) {
		t.Parallel()
		bad := good
		bad.ACSURL = "https://example.com/\x7f"
		if _, err := karakurisaml.New(bad, provisioner); err == nil {
			t.Fatal("an unparseable ACS URL was accepted")
		}
		bad = good
		bad.MetadataURL = "https://example.com/\x7f"
		if _, err := karakurisaml.New(bad, provisioner); err == nil {
			t.Fatal("an unparseable metadata URL was accepted")
		}
	})
}

func TestMetadataHandler(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	_, _, server := newProvider(t, idp, nil)

	resp, err := server.Client().Get(server.URL + "/saml/metadata")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Content-Type"); got != "application/samlmetadata+xml" {
		t.Errorf("Content-Type = %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "EntityDescriptor") {
		t.Fatalf("metadata does not look like SAML metadata: %s", body)
	}
	if !strings.Contains(string(body), server.URL+"/saml/acs") {
		t.Error("metadata does not publish the ACS URL, so an IdP could not deliver to it")
	}
}

func TestLoginRedirectsToIdP(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	_, _, server := newProvider(t, idp, nil)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(server.URL + "/saml/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	location := mustURL(t, resp.Header.Get("Location"))
	if !strings.HasPrefix(location.String(), idp.Server.URL+"/sso") {
		t.Errorf("Location = %q, want the IdP's SSO endpoint", location)
	}
	if location.Query().Get("SAMLRequest") == "" {
		t.Error("redirect carries no SAMLRequest")
	}

	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want the flow cookie", len(cookies))
	}
	if !cookies[0].HttpOnly {
		t.Error("flow cookie is readable by script")
	}
}

// The assertion arrives as a cross-site form POST, and browsers do not attach
// SameSite=Lax cookies to those. A Lax flow cookie would simply never arrive,
// and every login would be rejected as unsolicited.
func TestFlowCookieIsSameSiteNoneOverHTTPS(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	provisioner, _ := newProvisioner(t)

	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:    "https://karakuri.example.com/saml/metadata",
		MetadataURL: "https://karakuri.example.com/saml/metadata",
		ACSURL:      "https://karakuri.example.com/saml/acs",
		IDPMetadata: idp.Metadata(),
		StateKey:    []byte("k"),
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/login", nil))

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].SameSite != http.SameSiteNoneMode {
		t.Errorf("SameSite = %v, want None — Lax cookies do not accompany the IdP's cross-site POST", cookies[0].SameSite)
	}
	if !cookies[0].Secure {
		t.Error("SameSite=None requires Secure, or the browser drops the cookie")
	}
}

// samlResponseRE pulls the assertion out of the IdP's auto-submitting form,
// which is what a browser would post.
var samlResponseRE = regexp.MustCompile(`name="SAMLResponse" value="([^"]+)"`)

// login drives the whole flow: login redirect, IdP SSO, then the form POST back
// to the ACS endpoint, carrying cookies throughout.
func login(t *testing.T, p *karakurisaml.Provider, server *httptest.Server, mux *http.ServeMux) (auth.Principal, *http.Response, error) {
	t.Helper()

	var seen auth.Principal
	var lastErr error
	mux.Handle("/saml/acs", p.ACSHandler(
		func(w http.ResponseWriter, _ *http.Request, principal auth.Principal) {
			seen = principal
			_, _ = w.Write([]byte("welcome"))
		},
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			http.Error(w, err.Error(), http.StatusUnauthorized)
		},
	))

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// Follow the login redirect into the IdP, which answers with the form.
	resp, err := client.Get(server.URL + "/saml/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	match := samlResponseRE.FindSubmatch(body)
	if match == nil {
		t.Fatalf("IdP did not return a SAMLResponse form:\n%s", body)
	}

	// The value sits in an HTML attribute, so html/template has escaped it —
	// notably "+" as "&#43;", which is common in base64. A browser unescapes
	// before submitting; so must this.
	post, err := client.PostForm(server.URL+"/saml/acs", url.Values{
		"SAMLResponse": {html.UnescapeString(string(match[1]))},
	})
	if err != nil {
		t.Fatalf("post assertion: %v", err)
	}
	return seen, post, lastErr
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)

	provisioner, store := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       idp.Metadata(),
		RoleAttribute:     "eduPersonAffiliation",
		EmailAttribute:    "mail",
		NameAttribute:     "uid",
		StateKey:          []byte("a-state-signing-key"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idp.Register(p.ServiceProvider())
	mux.Handle("/saml/login", p.LoginHandler())

	principal, resp, lastErr := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (%v): %s", resp.StatusCode, lastErr, body)
	}
	if principal.ID != "saml:alice@example.com" {
		t.Fatalf("principal = %q, want the namespaced NameID", principal.ID)
	}
	if principal.Attrs["email"] != "alice@example.com" {
		t.Errorf("Attrs = %v, want the mail attribute carried through", principal.Attrs)
	}

	bindings, err := store.ListBindings(context.Background(), principal.ID)
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Role != "operator" {
		t.Fatalf("bindings = %+v, want one operator binding from eduPersonAffiliation", bindings)
	}
}

func TestRoundTripUnmappedGroupGrantsNothing(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	idp.Session.Groups = []string{"marketing"}

	provisioner, store := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       idp.Metadata(),
		RoleAttribute:     "eduPersonAffiliation",
		StateKey:          []byte("k"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idp.Register(p.ServiceProvider())
	mux.Handle("/saml/login", p.LoginHandler())

	principal, resp, _ := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the login to succeed", resp.StatusCode)
	}
	bindings, _ := store.ListBindings(context.Background(), principal.ID)
	if len(bindings) != 0 {
		t.Fatalf("bindings = %+v, want none — the group maps to no role", bindings)
	}
}

// Disabling an account has to outrank the identity provider still
// authenticating it — otherwise `krk auth users disable` is undone by the next
// login.
func TestRoundTripRefusesDisabledPrincipal(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)

	provisioner, store := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       idp.Metadata(),
		RoleAttribute:     "eduPersonAffiliation",
		StateKey:          []byte("k"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idp.Register(p.ServiceProvider())
	mux.Handle("/saml/login", p.LoginHandler())

	if err := store.PutPrincipal(context.Background(), auth.Principal{
		ID: "saml:alice@example.com", Disabled: true,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	_, resp, lastErr := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a disabled principal", resp.StatusCode)
	}
	if !errors.Is(lastErr, auth.ErrPrincipalDisabled) {
		t.Fatalf("err = %v, want ErrPrincipalDisabled", lastErr)
	}
}

func TestACSRejects(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	p, _, _ := newProvider(t, idp, nil)

	var lastErr error
	handler := p.ACSHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) { t.Error("onSuccess ran on a bad assertion") },
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			w.WriteHeader(http.StatusUnauthorized)
		},
	)

	cases := []struct {
		name string
		body string
	}{
		{name: "no assertion", body: ""},
		{name: "not base64", body: "SAMLResponse=!!!"},
		{name: "not XML", body: "SAMLResponse=aGVsbG8sIHdvcmxk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastErr = nil
			r := httptest.NewRequest(http.MethodPost, "/saml/acs", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if lastErr == nil {
				t.Error("onError was not told why")
			}
		})
	}
}

// Without onError, a bad assertion must still not reach the success path.
func TestACSDefaultErrorHandler(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	p, _, _ := newProvider(t, idp, nil)

	handler := p.ACSHandler(func(http.ResponseWriter, *http.Request, auth.Principal) {
		t.Error("onSuccess ran on a bad assertion")
	}, nil)

	r := httptest.NewRequest(http.MethodPost, "/saml/acs", strings.NewReader("SAMLResponse=!!!"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// An assertion signed by a different identity provider must not authenticate
// anybody, however well-formed it is.
func TestACSRejectsForeignIdP(t *testing.T) {
	t.Parallel()
	ours := newStubIdP(t)
	theirs := newStubIdP(t)

	provisioner, _ := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	// Configured to trust `ours`, but the flow is driven against `theirs`.
	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       ours.Metadata(),
		StateKey:          []byte("k"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux.Handle("/saml/login", p.LoginHandler())

	// Point a second provider at `theirs` purely to obtain a valid assertion
	// from the wrong issuer.
	impostor, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       theirs.Metadata(),
		StateKey:          []byte("k"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	theirs.Register(impostor.ServiceProvider())

	forged := assertionFrom(t, theirs, impostor, server)

	var lastErr error
	handler := p.ACSHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) {
			t.Error("a foreign assertion authenticated somebody")
		},
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			w.WriteHeader(http.StatusUnauthorized)
		},
	)
	r := httptest.NewRequest(http.MethodPost, "/saml/acs",
		strings.NewReader(url.Values{"SAMLResponse": {forged}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if lastErr == nil {
		t.Error("onError was not told why")
	}
}

// assertionFrom obtains a valid, signed assertion from an IdP.
func assertionFrom(t *testing.T, idp *stubIdP, p *karakurisaml.Provider, server *httptest.Server) string {
	t.Helper()

	mux := http.NewServeMux()
	own := httptest.NewServer(mux)
	t.Cleanup(own.Close)
	mux.Handle("/saml/login", p.LoginHandler())

	jar, _ := cookiejar.New(nil)
	resp, err := (&http.Client{Jar: jar}).Get(own.URL + "/saml/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	match := samlResponseRE.FindSubmatch(body)
	if match == nil {
		t.Fatalf("no SAMLResponse in:\n%s", body)
	}
	return html.UnescapeString(string(match[1]))
}

func TestAttributeLookupMatchesFriendlyName(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)

	// crewjam's IdP emits groups with Name "urn:oid:1.3.6.1.4.1.5923.1.1.1.1"
	// and FriendlyName "eduPersonAffiliation". Configuring the URI must work as
	// well as configuring the friendly name, because providers populate one,
	// the other, or both.
	provisioner, store := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	p, err := karakurisaml.New(karakurisaml.Config{
		EntityID:          server.URL + "/saml/metadata",
		MetadataURL:       server.URL + "/saml/metadata",
		ACSURL:            server.URL + "/saml/acs",
		IDPMetadata:       idp.Metadata(),
		RoleAttribute:     "urn:oid:1.3.6.1.4.1.5923.1.1.1.1",
		StateKey:          []byte("k"),
		InsecureAllowHTTP: true,
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idp.Register(p.ServiceProvider())
	mux.Handle("/saml/login", p.LoginHandler())

	principal, resp, lastErr := login(t, p, server, mux)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (%v)", resp.StatusCode, lastErr)
	}

	bindings, _ := store.ListBindings(context.Background(), principal.ID)
	if len(bindings) != 1 || bindings[0].Role != "operator" {
		t.Fatalf("bindings = %+v, want the URI-named attribute to resolve", bindings)
	}
}

func TestServiceProviderIsExposed(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t)
	p, _, server := newProvider(t, idp, nil)

	sp := p.ServiceProvider()
	if sp == nil {
		t.Fatal("ServiceProvider returned nil")
	}
	if sp.AcsURL.String() != server.URL+"/saml/acs" {
		t.Errorf("AcsURL = %q", sp.AcsURL.String())
	}
	var _ *crewjam.ServiceProvider = sp
}
