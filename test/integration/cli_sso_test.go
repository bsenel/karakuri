package integration_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// TestCLISSOLogin drives `krk auth login --sso` end to end.
//
// The browser is stood in for by an HTTP client that follows redirects, which
// is the only thing a browser contributes here: the interesting parts are the
// loopback handoff and the fact that what travels through it is unusable on its
// own.
func TestCLISSOLogin(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
	})
	defer cleanup()

	// The CLI caches credentials on disk; keep this run out of the developer's.
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))

	// --api-url includes the version prefix, so the client is built the same way.
	api := client.New(baseURL + "/api/v1")

	// The login URL is announced rather than opened: this test is the browser.
	urls := make(chan string, 1)
	done := make(chan error, 1)
	var session client.Session
	go func() {
		var err error
		session, err = api.SSOLogin(context.Background(), func(target string) { urls <- target }, false)
		done <- err
	}()

	// Read both: if the login fails before it announces a URL, waiting only on
	// urls would deadlock and report nothing useful.
	var loginURL string
	select {
	case loginURL = <-urls:
	case err := <-done:
		t.Fatalf("SSOLogin failed before announcing a URL: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("SSOLogin announced no URL")
	}

	if !strings.Contains(loginURL, "cli_port=") || !strings.Contains(loginURL, "cli_challenge=") {
		t.Fatalf("login URL carries no loopback details: %s", loginURL)
	}
	// The secret itself must never appear in the URL — only its hash.
	if strings.Contains(loginURL, "cli_verifier") {
		t.Error("the login URL carries the verifier")
	}

	// Play the browser. The redirect registered with the provider points at a
	// fixed public URL rather than the port this harness happened to bind, so
	// the provider's callback is replayed against the real address — the same
	// step the other federation tests take. Everything after that is followed
	// for real, including the hop to the loopback listener.
	browser := federatedClient(t)
	resp, err := browser.Get(loginURL)
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	_ = resp.Body.Close()

	callbackQuery := queryOf(t, resp.Header.Get("Location"))
	browser.CheckRedirect = nil // follow the loopback hop for real
	final, err := browser.Get(baseURL + "/api/v1/auth/sso/callback?" + callbackQuery)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = final.Body.Close()
	if final.StatusCode != http.StatusOK {
		t.Fatalf("loopback callback = %d, want 200", final.StatusCode)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SSOLogin: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("SSOLogin did not finish after the callback landed")
	}
	if session.PrincipalID != "oidc:alice-subject" {
		t.Fatalf("principal = %q, want the namespaced subject", session.PrincipalID)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("session carries no tokens")
	}

	// The credential works, and carries the role the group mapped to.
	me := decodeJSON(t, doJSON(t, session.AccessToken, http.MethodGet, baseURL+"/api/v1/auth/me", nil))
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != "operator" {
		t.Fatalf("roles = %v, want [operator]", roles)
	}

	// And it was cached, so the next krk invocation does not log in again.
	cached, ok := client2Session(t, baseURL+"/api/v1")
	if !ok {
		t.Fatal("the session was not cached")
	}
	if cached.RefreshToken != session.RefreshToken {
		t.Error("the cached session is not the one that was just established")
	}
}

// TestCLISSOExchangeNeedsTheVerifier is the property the whole handoff rests
// on: the code that travels through the browser is useless without the secret
// that never left the CLI.
func TestCLISSOExchangeNeedsTheVerifier(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
	})
	defer cleanup()

	code := captureHandoffCode(t, baseURL, karakuriauth.CLIChallenge("the-real-secret"))

	cases := []struct {
		name string
		body map[string]string
	}{
		{name: "wrong verifier", body: map[string]string{"code": code, "verifier": "not-the-secret"}},
		{name: "no verifier", body: map[string]string{"code": code}},
		{name: "no code", body: map[string]string{"verifier": "the-real-secret"}},
		{name: "forged code", body: map[string]string{"code": "made.up", "verifier": "the-real-secret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/sso/exchange", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Fatal("the exchange succeeded without the right verifier")
			}
		})
	}

	// The right one still works, so the refusals above are about the verifier
	// and not about the code being spent.
	ok := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/sso/exchange",
		map[string]string{"code": code, "verifier": "the-real-secret"})
	defer ok.Body.Close()
	assertStatus(t, ok, http.StatusOK)
}

// Redeeming twice must not yield two working sessions. The code carries a
// refresh token, refresh tokens rotate on first use, and presenting a spent one
// is treated as compromise.
func TestCLISSOCodeCannotBeReplayed(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
	})
	defer cleanup()

	code := captureHandoffCode(t, baseURL, karakuriauth.CLIChallenge("secret"))
	body := map[string]string{"code": code, "verifier": "secret"}

	first := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/sso/exchange", body)
	defer first.Body.Close()
	assertStatus(t, first, http.StatusOK)

	second := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/sso/exchange", body)
	defer second.Body.Close()
	if second.StatusCode == http.StatusOK {
		t.Fatal("a replayed handoff code produced a second session")
	}
}

func TestCLISSORejectsBadLoopbackPort(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, nil)
	defer cleanup()
	_ = idp

	for _, port := range []string{"0", "80", "70000", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			resp := mustGet(t, http.DefaultClient,
				baseURL+"/api/v1/auth/sso/login?cli_port="+port+"&cli_challenge=x")
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("cli_port=%s = %d, want 400", port, resp.StatusCode)
			}
		})
	}

	// A port with no challenge is refused too: without one the code would be
	// redeemable by whoever received it.
	resp := mustGet(t, http.DefaultClient, baseURL+"/api/v1/auth/sso/login?cli_port=9999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("cli_port with no challenge = %d, want 400", resp.StatusCode)
	}
}

// captureHandoffCode runs a browser login for a CLI-style flow and returns the
// code the server tried to hand to the loopback listener, without one running.
func captureHandoffCode(t *testing.T, baseURL, challenge string) string {
	t.Helper()

	client := federatedClient(t)
	// Stop at the loopback redirect rather than trying to connect to a port
	// nothing is listening on.
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Hostname() == "127.0.0.1" && req.URL.Path == "/callback" {
			return http.ErrUseLastResponse
		}
		if strings.HasPrefix(req.URL.Path, "/api/v1/auth/sso/callback") {
			return http.ErrUseLastResponse
		}
		return nil
	}

	resp, err := client.Get(baseURL + "/api/v1/auth/sso/login?cli_port=9999&cli_challenge=" + challenge)
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	_ = resp.Body.Close()

	// Replay the provider's callback against the real address, as the other
	// federation tests do.
	final, err := client.Get(baseURL + "/api/v1/auth/sso/callback?" + queryOf(t, resp.Header.Get("Location")))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = final.Body.Close()

	target := final.Header.Get("Location")
	if !strings.HasPrefix(target, "http://127.0.0.1:9999/callback?code=") {
		t.Fatalf("post-login redirect = %q, want the loopback handoff", target)
	}
	return strings.TrimPrefix(target, "http://127.0.0.1:9999/callback?code=")
}

// client2Session reads the CLI's on-disk credential cache.
func client2Session(t *testing.T, baseURL string) (client.Session, bool) {
	t.Helper()
	if _, err := os.Stat(client.CredentialsPath()); err != nil {
		return client.Session{}, false
	}
	return client.LoadSession(baseURL)
}

// queryOf returns the query string of a redirect target.
func queryOf(t *testing.T, location string) string {
	t.Helper()
	if location == "" {
		t.Fatal("expected a redirect, got none")
	}
	idx := strings.Index(location, "?")
	if idx < 0 {
		t.Fatalf("redirect %q carries no query", location)
	}
	return location[idx+1:]
}
