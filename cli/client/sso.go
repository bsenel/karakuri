package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// SSO login from a terminal.
//
// A browser finishes a federated login holding httpOnly cookies, which a
// command-line tool cannot read. So this opens a browser, listens on a loopback
// port, and lets the server hand the credential back through it.
//
// The code that comes back through the browser is useless on its own: a secret
// is generated here first, only its hash goes out in the login URL, and the
// secret has to be presented to redeem the code. Nothing usable ever passes
// through the browser, its history, or anything watching the URL bar.

// ssoTimeout bounds how long to wait for somebody to finish logging in. Long
// enough for a password manager and a second factor, short enough that a
// forgotten terminal does not hold a port forever.
const ssoTimeout = 5 * time.Minute

// ErrSSONotConfigured is returned when the server offers no federated login.
var ErrSSONotConfigured = errors.New("this server has no federated identity provider configured; use `krk auth login --id <you> --password-stdin`")

// SSOConfig is what /auth/sso/config reports.
type SSOConfig struct {
	Provider      string `json:"provider"`
	Enabled       bool   `json:"enabled"`
	PasswordLogin bool   `json:"password_login"`
	LoginURL      string `json:"login_url"`
}

// SSOConfig asks the server what kind of login it offers.
func (c *Client) SSOConfig() (SSOConfig, error) {
	data, status, err := c.do(http.MethodGet, "/auth/sso/config", nil)
	if err != nil {
		return SSOConfig{}, err
	}
	if status != http.StatusOK {
		return SSOConfig{}, fmt.Errorf("read login configuration: %s", apiMessage(data, status))
	}
	var out SSOConfig
	if err := json.Unmarshal(data, &out); err != nil {
		return SSOConfig{}, fmt.Errorf("decode login configuration: %w", err)
	}
	return out, nil
}

// SSOLogin runs the browser login and caches the resulting session.
//
// openBrowser may be nil, in which case the URL is only reported through
// announce — the flow a headless machine takes, where the operator opens the
// link somewhere else.
func (c *Client) SSOLogin(ctx context.Context, announce func(url string), openBrowser bool) (Session, error) {
	cfg, err := c.SSOConfig()
	if err != nil {
		return Session{}, err
	}
	if !cfg.Enabled {
		return Session{}, ErrSSONotConfigured
	}

	verifier, err := randomToken()
	if err != nil {
		return Session{}, err
	}

	// Bind the listener before opening the browser: the port has to be in the
	// URL, and a login that starts before anything is listening loses its
	// callback.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Session{}, fmt.Errorf("listen for the login callback: %w", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	// BaseURL already carries the API prefix (--api-url defaults to
	// http://localhost:8080/api/v1), so paths are appended to it directly, the
	// same way every other call in this package does it.
	loginURL := fmt.Sprintf("%s/auth/sso/login?cli_port=%d&cli_challenge=%s",
		c.BaseURL, port, url.QueryEscape(karakuriauth.CLIChallenge(verifier)))

	codes := make(chan string, 1)
	failures := make(chan error, 1)
	server := &http.Server{
		Handler:           http.HandlerFunc(callbackPage(codes, failures)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	if announce != nil {
		announce(loginURL)
	}
	if openBrowser {
		// A browser that will not open is not fatal: the URL has already been
		// printed, and somebody can paste it.
		_ = openInBrowser(loginURL)
	}

	ctx, cancel := context.WithTimeout(ctx, ssoTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return Session{}, fmt.Errorf("timed out waiting for the login to complete: %w", ctx.Err())
	case err := <-failures:
		return Session{}, err
	case code := <-codes:
		return c.redeem(code, verifier)
	}
}

// redeem exchanges the handoff code for a token pair and caches it.
func (c *Client) redeem(code, verifier string) (Session, error) {
	data, status, err := c.do(http.MethodPost, "/auth/sso/exchange", map[string]string{
		"code": code, "verifier": verifier,
	})
	if err != nil {
		return Session{}, err
	}
	if status != http.StatusOK {
		return Session{}, fmt.Errorf("complete login: %s", apiMessage(data, status))
	}

	var out struct {
		tokenResponse
		PrincipalID string `json:"principal_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Session{}, fmt.Errorf("decode token response: %w", err)
	}
	session := out.session(out.PrincipalID)
	if err := SaveSession(c.BaseURL, session); err != nil {
		return Session{}, fmt.Errorf("cache credentials: %w", err)
	}
	c.session = &session
	return session, nil
}

// callbackPage receives the browser's redirect and tells the person they can go
// back to their terminal.
func callbackPage(codes chan<- string, failures chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if reason := q.Get("error"); reason != "" {
			select {
			case failures <- fmt.Errorf("login failed: %s", reason):
			default:
			}
			http.Error(w, "Login failed. You can close this window.", http.StatusBadRequest)
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Not a login callback.", http.StatusBadRequest)
			return
		}
		select {
		case codes <- code:
		default:
			// A second callback for a login already completed. Nothing to do,
			// and the page should still be polite about it.
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(callbackHTML))
	}
}

const callbackHTML = `<!doctype html>
<meta charset="utf-8">
<title>Signed in</title>
<style>
  body { font: 16px/1.5 system-ui, sans-serif; margin: 4rem auto; max-width: 28rem; padding: 0 1rem; }
  p { color: #444; }
</style>
<h1>Signed in</h1>
<p>You can close this window and return to your terminal.</p>
`

// randomToken returns the secret the handoff code is bound to. Its only
// requirement is that nobody can predict it, so crypto/rand is the only
// acceptable source and its error is returned rather than ignored.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate login secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// openInBrowser is best effort by design — see SSOLogin.
func openInBrowser(target string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	return exec.Command(cmd, append(args, target)...).Start()
}
