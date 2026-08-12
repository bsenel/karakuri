package handler

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// CLI login, and why it is not just "read the cookie".
//
// A browser finishes a federated login holding httpOnly cookies, which is
// exactly what a command-line tool cannot read. So `krk auth login --sso` opens
// a browser, listens on a loopback port, and the server hands the credential
// back through that port.
//
// The handoff is a URL, and a URL lands in browser history. So the code in it
// is useless on its own: the CLI generates a secret before opening the browser,
// sends only its hash, and must present the secret to redeem the code. That is
// PKCE's argument applied to our own handoff, and it means the browser — and
// anything reading over its shoulder — never sees a usable credential.
//
// The code is sealed rather than stored, so any replica can redeem one another
// started. Nothing enforces single use, and nothing needs to: the code carries
// a refresh token, refresh tokens rotate on first use, and presenting a spent
// one triggers the reuse detection that revokes the whole family. A replayed
// code does not grant access, it ends the session it was stolen from.

const (
	// cliPortParam and cliChallengeParam are what `krk auth login --sso` adds
	// to the login URL.
	cliPortParam      = "cli_port"
	cliChallengeParam = "cli_challenge"

	// cliFlowCookie carries the loopback details across the identity
	// provider's redirect.
	cliFlowCookie = "karakuri_cli_login"

	// cliFlowTTL bounds how long a CLI login may take, and cliCodeTTL how long
	// the resulting code may sit unredeemed. The CLI redeems immediately, so
	// this is short on purpose.
	cliFlowTTL = 10 * time.Minute
	cliCodeTTL = 2 * time.Minute
)

// cliFlow is what survives the round trip through the identity provider.
type cliFlow struct {
	Port      int    `json:"p"`
	Challenge string `json:"c"`
}

// cliCode is the sealed handoff the browser carries to the loopback listener.
type cliCode struct {
	Refresh   string `json:"r"`
	Principal string `json:"s"`
	Challenge string `json:"c"`
}

// beginCLILogin records the loopback details when a login came from the CLI.
//
// The port is validated rather than trusted, and the host is never taken from
// the request at all — the redirect target is always 127.0.0.1. A "send the
// credential wherever this parameter says" endpoint is an open redirect with a
// token attached, which is the worst kind.
func (h *SSOHandler) beginCLILogin(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get(cliPortParam)
	if raw == "" {
		return nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1024 || port > 65535 {
		return fmt.Errorf("%s must be a port between 1024 and 65535", cliPortParam)
	}
	challenge := r.URL.Query().Get(cliChallengeParam)
	if challenge == "" {
		return fmt.Errorf("%s is required alongside %s", cliChallengeParam, cliPortParam)
	}

	sealed, err := h.Federation.Sealer.Seal(cliFlow{Port: port, Challenge: challenge}, cliFlowTTL)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cliFlowCookie,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		Secure:   !h.InsecureAllowHTTP,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cliFlowTTL.Seconds()),
	})
	return nil
}

// cliRedirect returns where to send the browser when this login came from the
// CLI, or "" when it did not.
func (h *SSOHandler) cliRedirect(w http.ResponseWriter, r *http.Request, p auth.Principal, pair auth.TokenPair) string {
	cookie, err := r.Cookie(cliFlowCookie)
	if err != nil {
		return ""
	}
	// However this ends, the flow is over.
	http.SetCookie(w, &http.Cookie{Name: cliFlowCookie, Path: "/", MaxAge: -1})

	var flow cliFlow
	if err := h.Federation.Sealer.Open(cookie.Value, &flow); err != nil {
		return ""
	}
	code, err := h.Federation.Sealer.Seal(cliCode{
		Refresh:   pair.RefreshToken,
		Principal: p.ID,
		Challenge: flow.Challenge,
	}, cliCodeTTL)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/callback?code=%s", flow.Port, code)
}

// Exchange redeems a CLI handoff code for the token pair it stands for.
//
// POST /api/v1/auth/sso/exchange  {"code": "...", "verifier": "..."}
func (h *SSOHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	if body.Code == "" || body.Verifier == "" {
		authError(w, http.StatusBadRequest, "bad_request", "code and verifier are required")
		return
	}

	var code cliCode
	if err := h.Federation.Sealer.Open(body.Code, &code); err != nil {
		authError(w, http.StatusUnauthorized, "invalid_code", "code is not valid")
		return
	}
	if subtle.ConstantTimeCompare([]byte(code.Challenge), []byte(karakuriauth.CLIChallenge(body.Verifier))) != 1 {
		authError(w, http.StatusUnauthorized, "invalid_code", "code is not valid")
		return
	}

	// The code carries a refresh token, so redeeming it rotates: the CLI ends
	// up with a pair nobody else ever held, and a replay of the same code
	// presents a spent token and revokes the family.
	pair, err := h.Tokens.IssueForRefresh(r.Context(), code.Refresh)
	if err != nil {
		authError(w, http.StatusUnauthorized, "invalid_code", "code is not valid")
		return
	}
	writeJSON(w, map[string]any{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"principal_id":  code.Principal,
	})
}
