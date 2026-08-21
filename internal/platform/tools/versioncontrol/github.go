package versioncontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHub is a VersionControlAdapter backed by the GitHub REST API.
// It depends only on net/http; no SDK.
type GitHub struct {
	token  string
	repo   string // "owner/name" — used when caller does not pass an explicit repo
	client *http.Client
}

func NewGitHub(token, defaultRepo string) *GitHub {
	return &GitHub{
		token:  token,
		repo:   defaultRepo,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) Active() bool { return g.token != "" }

func (g *GitHub) CreatePR(ctx context.Context, pr PullRequest) (string, error) {
	repo := g.repo
	if repo == "" {
		return "", fmt.Errorf("github: no repo configured")
	}
	body := map[string]string{
		"title": pr.Title,
		"body":  pr.Body,
		"head":  pr.HeadBranch,
		"base":  pr.BaseBranch,
	}
	var resp struct {
		HTMLURL string `json:"html_url"`
		Message string `json:"message"`
	}
	if err := g.do(ctx, "POST", fmt.Sprintf("/repos/%s/pulls", repo), body, &resp); err != nil {
		return "", err
	}
	if resp.HTMLURL == "" {
		return "", fmt.Errorf("github: PR create failed: %s", resp.Message)
	}
	return resp.HTMLURL, nil
}

func (g *GitHub) ListPRs(ctx context.Context, repo string, since time.Time) ([]PRSummary, error) {
	if repo == "" {
		repo = g.repo
	}
	if repo == "" {
		return nil, fmt.Errorf("github: no repo configured")
	}
	var raw []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		HTMLURL   string    `json:"html_url"`
		UpdatedAt time.Time `json:"updated_at"`
		Head      struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/pulls?state=open&per_page=50", repo), nil, &raw); err != nil {
		return nil, err
	}
	out := make([]PRSummary, 0, len(raw))
	for _, r := range raw {
		if !since.IsZero() && r.UpdatedAt.Before(since) {
			continue
		}
		s := PRSummary{ID: fmt.Sprintf("%d", r.Number), Title: r.Title, URL: r.HTMLURL}
		s.CheckState, s.FailingChecks = g.checkState(ctx, repo, r.Head.SHA)
		out = append(out, s)
	}
	return out, nil
}

// checkState reads the combined CI verdict for one commit.
//
// Deferred from Phase 22 and picked up in Phase 25, so that "what is currently
// broken" is evidence the pack can read rather than something an operator has
// to relay. Failures here are silent by design: a missing verdict is reported
// as an empty state, and degrading a list of open pull requests into an error
// because the checks endpoint was slow would lose the part that always works.
func (g *GitHub) checkState(ctx context.Context, repo, sha string) (string, []string) {
	if sha == "" {
		return "", nil
	}
	var raw struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=100", repo, sha), nil, &raw); err != nil {
		return "", nil
	}
	if len(raw.CheckRuns) == 0 {
		return "", nil
	}

	var failing []string
	pending := false
	for _, c := range raw.CheckRuns {
		if c.Status != "completed" {
			pending = true
			continue
		}
		switch c.Conclusion {
		case "success", "neutral", "skipped":
			// Not a failure. "skipped" in particular is how a conditional job
			// reports not running, and counting it red would make every PR red.
		default:
			failing = append(failing, c.Name)
		}
	}
	switch {
	case len(failing) > 0:
		// Red wins over pending: a run still going cannot un-fail what already
		// failed, and waiting to say so delays the only actionable verdict.
		return "failure", failing
	case pending:
		return "pending", nil
	default:
		return "success", nil
	}
}

func (g *GitHub) GetCommits(ctx context.Context, repo string, since time.Time) ([]Commit, error) {
	if repo == "" {
		repo = g.repo
	}
	if repo == "" {
		return nil, fmt.Errorf("github: no repo configured")
	}
	path := fmt.Sprintf("/repos/%s/commits?per_page=50", repo)
	if !since.IsZero() {
		path += "&since=" + since.UTC().Format(time.RFC3339)
	}
	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := g.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw))
	for _, r := range raw {
		out = append(out, Commit{
			SHA:     r.SHA,
			Message: strings.SplitN(r.Commit.Message, "\n", 2)[0],
			Author:  r.Commit.Author.Name,
			Date:    r.Commit.Author.Date,
		})
	}
	return out, nil
}

func (g *GitHub) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github: %s %s -> %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
