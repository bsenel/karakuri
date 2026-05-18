package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080/api/v1"
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

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
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *Client) Get(path string) ([]byte, int, error)  { return c.do(http.MethodGet, path, nil) }
func (c *Client) Post(path string, body any) ([]byte, int, error) {
	return c.do(http.MethodPost, path, body)
}
func (c *Client) Delete(path string) ([]byte, int, error) { return c.do(http.MethodDelete, path, nil) }

func (c *Client) CreateSession(mode, input, parentSHA string) (map[string]any, error) {
	data, code, err := c.Post("/sessions", map[string]string{
		"mode": mode, "input": input, "parent_sha": parentSHA,
	})
	if err != nil || code >= 400 {
		return nil, fmt.Errorf("create session: %s %w", string(data), err)
	}
	var out map[string]any
	return out, json.Unmarshal(data, &out)
}

func (c *Client) RunSession(sha string) error {
	_, code, err := c.Post("/sessions/"+sha+"/run", nil)
	if err != nil || code >= 400 {
		return fmt.Errorf("run session failed: %d", code)
	}
	return nil
}

func (c *Client) WaitForCompletion(sha string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, _, err := c.Get("/sessions/" + sha + "/status")
		if err != nil {
			return err
		}
		var st map[string]string
		_ = json.Unmarshal(data, &st)
		if st["state"] == "completed" || st["state"] == "failed" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for session %s", sha)
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
