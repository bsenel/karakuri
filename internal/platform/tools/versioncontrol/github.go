package versioncontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub is a VersionControlAdapter backed by the GitHub REST API.
// It depends only on net/http; no SDK.
type GitHub struct {
	token  string
	repo   string // "owner/name" — used when caller does not pass an explicit repo
	client *http.Client

	// ownAuthors holds the logins, lower-cased, whose pull requests are the
	// deployment's own writing. GitHub logins are case-insensitive.
	ownAuthors map[string]bool
}

// NewGitHub builds the adapter. ownAuthors names the logins this deployment
// opens pull requests as — its bot account, its operator, dependabot — and may
// be empty, in which case no pull request is marked as the deployment's own.
func NewGitHub(token, defaultRepo string, ownAuthors ...string) *GitHub {
	own := make(map[string]bool, len(ownAuthors))
	for _, a := range ownAuthors {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			own[a] = true
		}
	}
	return &GitHub{
		token:      token,
		repo:       defaultRepo,
		client:     &http.Client{Timeout: 30 * time.Second},
		ownAuthors: own,
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
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Head      struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/pulls?state=open&per_page=50", repo), nil, &raw); err != nil {
		return nil, err
	}
	out := make([]PRSummary, 0, len(raw))
	for _, r := range raw {
		if !since.IsZero() && r.UpdatedAt.Before(since) {
			continue
		}
		s := PRSummary{
			ID: fmt.Sprintf("%d", r.Number), Title: r.Title, URL: r.HTMLURL,
			Author:     r.User.Login,
			OwnAuthor:  g.ownAuthors[strings.ToLower(r.User.Login)],
			HeadBranch: r.Head.Ref, HeadSHA: r.Head.SHA,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
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

// ListChecks reads the check runs on ref. Only check runs: a repository that
// reports CI through the older commit statuses has none, which a verifier
// reads as no data rather than as green.
func (g *GitHub) ListChecks(ctx context.Context, repo, ref string) ([]CheckRun, error) {
	if repo == "" {
		repo = g.repo
	}
	if repo == "" {
		return nil, fmt.Errorf("github: no repo configured")
	}
	var raw struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Name       string `json:"name"`
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	path := fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=100", repo, url.PathEscape(ref))
	if err := g.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	// A partial list could leave out the one failing check, so more than one
	// page is an error rather than a verdict over the first.
	if raw.TotalCount > len(raw.CheckRuns) {
		return nil, fmt.Errorf("github: checks: %d check runs on %s, more than one page", raw.TotalCount, ref)
	}
	out := make([]CheckRun, 0, len(raw.CheckRuns))
	for _, c := range raw.CheckRuns {
		out = append(out, CheckRun{Name: c.Name, HeadSHA: c.HeadSHA, Status: c.Status, Conclusion: c.Conclusion})
	}
	return out, nil
}

// changedFilesMaxPages is GitHub's own limit: the files endpoint lists at most
// 3000 files, a hundred a page.
const changedFilesMaxPages = 30

// ChangedFiles reads the paths a pull request's diff touches, every page.
func (g *GitHub) ChangedFiles(ctx context.Context, repo, prID string) ([]string, error) {
	if repo == "" {
		repo = g.repo
	}
	if repo == "" {
		return nil, fmt.Errorf("github: no repo configured")
	}
	var out []string
	for page := 1; page <= changedFilesMaxPages; page++ {
		var raw []struct {
			Filename string `json:"filename"`
		}
		path := fmt.Sprintf("/repos/%s/pulls/%s/files?per_page=100&page=%d", repo, url.PathEscape(prID), page)
		if err := g.do(ctx, "GET", path, nil, &raw); err != nil {
			return nil, err
		}
		for _, f := range raw {
			out = append(out, f.Filename)
		}
		if len(raw) < 100 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("github: changed files: pull request %s touches more files than GitHub lists", prID)
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
