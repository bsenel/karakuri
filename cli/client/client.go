package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is a thin HTTP client for the Karakuri API.
//
// Authentication is handled here rather than by each command: access tokens
// last minutes, so every call resolves a fresh one from the cached session,
// refreshing when it is close to expiry.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	// session caches the credential for the process lifetime. Refresh tokens
	// rotate on every use, so holding one in memory avoids re-reading (and
	// re-spending) the file on each call within a single command.
	session *Session
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080/api/v1"
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// do issues an unauthenticated request. Used for the login and refresh
// endpoints, where the credential is the body.
func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// doAuth issues an authenticated request, attaching a valid access token.
func (c *Client) doAuth(method, path string, body any) ([]byte, int, error) {
	token, err := c.accessToken()
	if err != nil {
		return nil, 0, err
	}
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	// A 403 is a policy decision, not a transport failure — surface the
	// server's reason rather than an empty body and a status code.
	if resp.StatusCode == http.StatusForbidden {
		return data, resp.StatusCode, errors.New(apiMessage(data, resp.StatusCode))
	}
	return data, resp.StatusCode, nil
}

func (c *Client) Get(path string) ([]byte, int, error) { return c.doAuth(http.MethodGet, path, nil) }
func (c *Client) Post(path string, body any) ([]byte, int, error) {
	return c.doAuth(http.MethodPost, path, body)
}
func (c *Client) Put(path string, body any) ([]byte, int, error) {
	return c.doAuth(http.MethodPut, path, body)
}
func (c *Client) Delete(path string) ([]byte, int, error) {
	return c.doAuth(http.MethodDelete, path, nil)
}

// GetPublic issues an unauthenticated GET, for the endpoints that allow it
// (/health). Used by `krk` before a session exists.
func (c *Client) GetPublic(path string) ([]byte, int, error) {
	return c.do(http.MethodGet, path, nil)
}

func PrintOutput(data []byte, format string) {
	switch format {
	case "quiet":
		return
	case "json":
		os.Stdout.Write(data)
		os.Stdout.Write([]byte("\n"))
	default:
		var v any
		_ = json.Unmarshal(data, &v)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
	}
}
