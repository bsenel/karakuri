package design

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Figma is a DesignAdapter backed by the Figma REST API.
// Auth uses a Personal Access Token via the X-Figma-Token header.
type Figma struct {
	token  string
	client *http.Client
}

func NewFigma(token string) *Figma {
	return &Figma{token: token, client: &http.Client{Timeout: 30 * time.Second}}
}

func (f *Figma) Name() string { return "figma" }

func (f *Figma) Active() bool { return f.token != "" }

func (f *Figma) GetFile(ctx context.Context, id string) (DesignFile, error) {
	if id == "" {
		return DesignFile{}, fmt.Errorf("figma: file id required")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.figma.com/v1/files/"+id, nil)
	if err != nil {
		return DesignFile{}, err
	}
	req.Header.Set("X-Figma-Token", f.token)

	resp, err := f.client.Do(req)
	if err != nil {
		return DesignFile{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return DesignFile{}, fmt.Errorf("figma: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var raw struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return DesignFile{}, fmt.Errorf("figma: malformed response: %w", err)
	}
	return DesignFile{
		ID:   id,
		Name: raw.Name,
		URL:  fmt.Sprintf("https://www.figma.com/file/%s", id),
	}, nil
}
