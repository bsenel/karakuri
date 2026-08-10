package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ErrNotLoggedIn is returned when no usable credential is cached.
var ErrNotLoggedIn = errors.New("not logged in — run `krk auth login`")

// Session is one API server's cached credentials.
type Session struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	PrincipalID  string    `json:"principal_id,omitempty"`
}

// store is the on-disk shape: one entry per API URL, so a single machine can
// hold credentials for several servers at once.
type store struct {
	Sessions map[string]Session `json:"sessions"`
}

// refreshSkew triggers a refresh slightly before the access token actually
// expires, so a long-running command does not fail mid-flight.
const refreshSkew = 60 * time.Second

// CredentialsPath returns the credential cache location, honouring
// KARAKURI_CREDENTIALS for tests and for operators who keep it elsewhere.
func CredentialsPath() string {
	if p := os.Getenv("KARAKURI_CREDENTIALS"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = ".config"
	}
	return filepath.Join(dir, "karakuri", "credentials.json")
}

func loadStore() store {
	s := store{Sessions: map[string]Session{}}
	data, err := os.ReadFile(CredentialsPath())
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil || s.Sessions == nil {
		return store{Sessions: map[string]Session{}}
	}
	return s
}

func saveStore(s store) error {
	path := CredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// 0600: the refresh token in here is a live credential.
	return os.WriteFile(path, data, 0o600)
}

// SaveSession caches credentials for one API URL.
func SaveSession(apiURL string, s Session) error {
	all := loadStore()
	all.Sessions[apiURL] = s
	return saveStore(all)
}

// LoadSession reads the cached credentials for one API URL.
func LoadSession(apiURL string) (Session, bool) {
	s, ok := loadStore().Sessions[apiURL]
	return s, ok && s.AccessToken != ""
}

// ClearSession forgets one API URL's credentials.
func ClearSession(apiURL string) error {
	all := loadStore()
	delete(all.Sessions, apiURL)
	return saveStore(all)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (t tokenResponse) session(principalID string) Session {
	return Session{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(t.ExpiresIn) * time.Second),
		PrincipalID:  principalID,
	}
}

// Login exchanges a password for a token pair and caches it.
func (c *Client) Login(id, password string) (Session, error) {
	data, status, err := c.do(http.MethodPost, "/auth/token", map[string]string{"id": id, "password": password})
	if err != nil {
		return Session{}, err
	}
	if status != http.StatusOK {
		return Session{}, fmt.Errorf("login failed: %s", apiMessage(data, status))
	}
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return Session{}, fmt.Errorf("decode token response: %w", err)
	}
	session := tr.session(id)
	if err := SaveSession(c.BaseURL, session); err != nil {
		return Session{}, fmt.Errorf("cache credentials: %w", err)
	}
	c.session = &session
	return session, nil
}

// LoginWithRefreshToken adopts a refresh token minted elsewhere — the flow a
// service account uses, since it never has a password.
func (c *Client) LoginWithRefreshToken(refreshToken string) (Session, error) {
	session, err := c.refresh(Session{RefreshToken: refreshToken})
	if err != nil {
		return Session{}, err
	}
	c.session = &session
	return session, nil
}

// Logout revokes the cached session server-side and forgets it locally.
func (c *Client) Logout() error {
	session, ok := LoadSession(c.BaseURL)
	if ok && session.RefreshToken != "" {
		// Best effort: the local credential is going either way.
		_, _, _ = c.do(http.MethodPost, "/auth/revoke", map[string]string{"refresh_token": session.RefreshToken})
	}
	c.session = nil
	return ClearSession(c.BaseURL)
}

// refresh rotates a refresh token and caches the resulting pair.
func (c *Client) refresh(session Session) (Session, error) {
	if session.RefreshToken == "" {
		return Session{}, ErrNotLoggedIn
	}
	data, status, err := c.do(http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": session.RefreshToken})
	if err != nil {
		return Session{}, err
	}
	if status != http.StatusOK {
		// The cached credential is dead; drop it so the next command says
		// "log in" rather than replaying a token the server has revoked.
		_ = ClearSession(c.BaseURL)
		return Session{}, fmt.Errorf("%w: %s", ErrNotLoggedIn, apiMessage(data, status))
	}
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return Session{}, fmt.Errorf("decode refresh response: %w", err)
	}
	next := tr.session(session.PrincipalID)
	if err := SaveSession(c.BaseURL, next); err != nil {
		return Session{}, fmt.Errorf("cache credentials: %w", err)
	}
	return next, nil
}

// accessToken returns a usable access token, refreshing if the cached one is
// close to expiry. Refresh tokens rotate on every use, so the result is written
// back before it is used.
func (c *Client) accessToken() (string, error) {
	if c.session == nil {
		session, ok := LoadSession(c.BaseURL)
		if !ok {
			return "", ErrNotLoggedIn
		}
		c.session = &session
	}
	if time.Now().Before(c.session.ExpiresAt.Add(-refreshSkew)) {
		return c.session.AccessToken, nil
	}
	next, err := c.refresh(*c.session)
	if err != nil {
		c.session = nil
		return "", err
	}
	c.session = &next
	return next.AccessToken, nil
}

// AccessToken returns a valid access token for callers that manage their own
// HTTP request — the SSE streamer, which needs the header on a long-lived
// connection the shared client does not own.
func (c *Client) AccessToken() (string, error) { return c.accessToken() }

// apiMessage extracts the server's reason from an error body, falling back to
// the status code when the body is not the shape we expect.
func apiMessage(data []byte, status int) string {
	var body struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(data, &body); err == nil {
		switch {
		case body.Reason != "" && body.Error != "":
			return body.Error + ": " + body.Reason
		case body.Error != "":
			return body.Error
		}
	}
	if len(data) > 0 {
		return fmt.Sprintf("HTTP %d: %s", status, data)
	}
	return fmt.Sprintf("HTTP %d", status)
}
