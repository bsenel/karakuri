package versioncontrol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// toServer sends every request the adapter makes to srv instead of
// api.github.com, keeping the path and query.
type toServer struct{ srv *httptest.Server }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	u, _ := url.Parse(t.srv.URL)
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(r)
}

// The author travels on the summary, and OwnAuthor is set only for the logins
// the operator listed — compared without case, as GitHub compares logins.
func TestListPRsCarriesTheAuthorAndWhetherItIsTheDeploymentsOwn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/check-runs") {
			_, _ = w.Write([]byte(`{"check_runs":[]}`))
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`[
			{"number":1,"title":"ours","updated_at":"` + now + `","user":{"login":"Karakuri-Bot"}},
			{"number":2,"title":"bump","updated_at":"` + now + `","user":{"login":"dependabot[bot]"}},
			{"number":3,"title":"theirs","updated_at":"` + now + `","user":{"login":"stranger"}}
		]`))
	}))
	defer srv.Close()

	g := NewGitHub("t", "acme/api", " karakuri-bot ", "dependabot[bot]", "")
	g.client = &http.Client{Transport: toServer{srv}}

	prs, err := g.ListPRs(context.Background(), "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		author string
		own    bool
	}{{"Karakuri-Bot", true}, {"dependabot[bot]", true}, {"stranger", false}}
	if len(prs) != len(want) {
		t.Fatalf("got %d pull requests, want %d", len(prs), len(want))
	}
	for i, w := range want {
		if prs[i].Author != w.author || prs[i].OwnAuthor != w.own {
			t.Errorf("PR %s: author %q own %v, want %q own %v", prs[i].ID, prs[i].Author, prs[i].OwnAuthor, w.author, w.own)
		}
	}

	// No list configured: nothing is the deployment's own.
	plain := NewGitHub("t", "acme/api")
	plain.client = &http.Client{Transport: toServer{srv}}
	prs, err = plain.ListPRs(context.Background(), "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range prs {
		if pr.OwnAuthor {
			t.Errorf("PR %s by %q is marked own with no own_authors configured", pr.ID, pr.Author)
		}
	}
}

// The pull request carries what the stream verifiers read: its head branch and
// commit, and when it was opened and last updated.
func TestListPRsCarriesTheHeadAndTimes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/check-runs") {
			_, _ = w.Write([]byte(`{"check_runs":[]}`))
			return
		}
		_, _ = w.Write([]byte(`[{"number":7,"created_at":"2026-10-16T09:00:00Z","updated_at":"2026-10-17T10:00:00Z",
			"head":{"ref":"karakuri/ux","sha":"c2"},"user":{"login":"x"}}]`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	prs, err := g.ListPRs(context.Background(), "", time.Time{})
	if err != nil || len(prs) != 1 {
		t.Fatalf("prs %v, err %v", prs, err)
	}
	pr := prs[0]
	if pr.HeadBranch != "karakuri/ux" || pr.HeadSHA != "c2" ||
		!pr.CreatedAt.Equal(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)) ||
		!pr.UpdatedAt.Equal(time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("got %+v", pr)
	}
}

// Every check run on the ref, with the commit it ran on; more than one page is
// an error, because the page left out could hold the failing check.
func TestListChecks(t *testing.T) {
	body := `{"total_count":2,"check_runs":[
		{"id":11,"name":"build","head_sha":"c2","status":"completed","conclusion":"success"},
		{"id":12,"name":"test","head_sha":"c2","status":"in_progress","conclusion":null}]}`
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	checks, err := g.ListChecks(context.Background(), "", "c2")
	if err != nil {
		t.Fatal(err)
	}
	want := []CheckRun{
		{ID: 11, Name: "build", HeadSHA: "c2", Status: "completed", Conclusion: "success"},
		{ID: 12, Name: "test", HeadSHA: "c2", Status: "in_progress"},
	}
	if gotPath != "/repos/acme/api/commits/c2/check-runs" || len(checks) != 2 || checks[0] != want[0] || checks[1] != want[1] {
		t.Errorf("path %s, checks %+v", gotPath, checks)
	}

	body = `{"total_count":101,"check_runs":[{"name":"build","head_sha":"c2","status":"completed","conclusion":"success"}]}`
	if _, err := g.ListChecks(context.Background(), "", "c2"); err == nil {
		t.Error("a partial page of checks was returned as the whole list")
	}
}

// Every changed path, across pages.
func TestChangedFilesReadsEveryPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/api/pulls/7/files" {
			http.NotFound(w, r)
			return
		}
		page := r.URL.Query().Get("page")
		n := 100
		if page == "2" {
			n = 1
		}
		files := make([]string, 0, n)
		for range n {
			files = append(files, `{"filename":"docs/p`+page+`.md"}`)
		}
		_, _ = w.Write([]byte("[" + strings.Join(files, ",") + "]"))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	files, err := g.ChangedFiles(context.Background(), "", "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 101 || files[100] != "docs/p2.md" {
		t.Errorf("got %d files: %v", len(files), files)
	}
}

// A rename touches the path the file left as well as the one it arrived at, so
// moving a file out of a path is not hidden behind its new name.
func TestChangedFilesNamesARenamesOldPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"filename":"docs/x.go","previous_filename":"internal/x.go"},{"filename":"docs/y.md"}]`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	files, err := g.ChangedFiles(context.Background(), "", "7")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/x.go", "internal/x.go", "docs/y.md"}
	if !slices.Equal(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
}

// What the delivery stream reads to mark a group ready (Phase 44 step 1): the
// body holding the group's checklist, the base it is stacked on, whether it is
// still a draft, and the node ID GitHub's GraphQL API needs to change that.
func TestListPRsCarriesTheBodyBaseDraftAndNodeID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/check-runs") {
			_, _ = w.Write([]byte(`{"check_runs":[]}`))
			return
		}
		_, _ = w.Write([]byte(`[{"number":244,"node_id":"PR_kwDOAbc123","title":"feat(software): group 2",
			"body":"## What changed\n- [x] Step 1a\n- [ ] Step 1b","draft":true,
			"updated_at":"2026-11-14T10:00:00Z",
			"head":{"ref":"karakuri/phase-44-group-2","sha":"c3"},
			"base":{"ref":"karakuri/phase-44-group-1","sha":"c1"},
			"user":{"login":"x"}}]`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	prs, err := g.ListPRs(context.Background(), "", time.Time{})
	if err != nil || len(prs) != 1 {
		t.Fatalf("prs %v, err %v", prs, err)
	}
	pr := prs[0]
	if pr.Number != 244 {
		t.Errorf("Number = %d, want 244", pr.Number)
	}
	if pr.Body != "## What changed\n- [x] Step 1a\n- [ ] Step 1b" {
		t.Errorf("Body = %q", pr.Body)
	}
	if pr.BaseBranch != "karakuri/phase-44-group-1" {
		t.Errorf("BaseBranch = %q, want the base ref", pr.BaseBranch)
	}
	if !pr.Draft {
		t.Error("Draft = false for a draft pull request")
	}
	if pr.NodeID != "PR_kwDOAbc123" {
		t.Errorf("NodeID = %q, want PR_kwDOAbc123", pr.NodeID)
	}
}

// The commits on a delivery branch since its base (Phase 44 step 3): one
// compare call from base to head, each commit's full SHA, subject line,
// author and date, oldest first as GitHub lists them.
func TestCompareCommitsReadsTheCommitsFromBaseToHead(t *testing.T) {
	var gotPath string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ahead","ahead_by":2,"total_commits":2,"commits":[
			{"sha":"a0a3ad28c5f4238900b6b9476156e77ff3a80e05","commit":{"message":"test(software): specify the checklist\n\nSlice A.",
				"author":{"name":"Ada","date":"2026-11-14T17:30:14Z"}}},
			{"sha":"b2dca659d49bc624890076218fe35ee284da7a15","commit":{"message":"feat(software): show the checklist",
				"author":{"name":"Grace","date":"2026-11-14T17:43:18Z"}}}]}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	commits, err := g.CompareCommits(context.Background(), "", "karakuri/phase-44-mark-ready", "karakuri/phase-44-step-03")
	if err != nil {
		t.Fatalf("CompareCommits: %v", err)
	}
	if want := "/repos/acme/api/compare/karakuri/phase-44-mark-ready...karakuri/phase-44-step-03"; gotPath != want {
		t.Errorf("path = %s, want %s", gotPath, want)
	}
	want := []Commit{
		{SHA: "a0a3ad28c5f4238900b6b9476156e77ff3a80e05", Message: "test(software): specify the checklist",
			Author: "Ada", Date: time.Date(2026, 11, 14, 17, 30, 14, 0, time.UTC)},
		{SHA: "b2dca659d49bc624890076218fe35ee284da7a15", Message: "feat(software): show the checklist",
			Author: "Grace", Date: time.Date(2026, 11, 14, 17, 43, 18, 0, time.UTC)},
	}
	if len(commits) != len(want) {
		t.Fatalf("got %d commits, want %d: %+v", len(commits), len(want), commits)
	}
	for i, w := range want {
		c := commits[i]
		if c.SHA != w.SHA || c.Message != w.Message || c.Author != w.Author || !c.Date.Equal(w.Date) {
			t.Errorf("commit %d = %+v, want %+v", i, c, w)
		}
	}

	// A base or head the forge does not know is an error, not "no commits".
	status = http.StatusNotFound
	if commits, err := g.CompareCommits(context.Background(), "", "main", "gone"); err == nil {
		t.Errorf("a 404 from compare was read as %d commits", len(commits))
	}
}

// markReadyServer answers the pull request read with nodeID and the GraphQL
// POST with graphqlStatus and graphqlBody, recording every request in order.
type markReadyServer struct {
	nodeID        string
	getStatus     int
	graphqlStatus int
	graphqlBody   string

	requests []string // "METHOD path"
	auth     []string
	mutation string
}

func (m *markReadyServer) start(t *testing.T) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.requests = append(m.requests, r.Method+" "+r.URL.Path)
		m.auth = append(m.auth, r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/api/pulls/244":
			if m.getStatus != 0 {
				w.WriteHeader(m.getStatus)
				_, _ = w.Write([]byte(`{"message":"Not Found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"number":244,"node_id":"` + m.nodeID + `","draft":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			b, _ := io.ReadAll(r.Body)
			m.mutation = string(b)
			if m.graphqlStatus != 0 {
				w.WriteHeader(m.graphqlStatus)
			}
			_, _ = w.Write([]byte(m.graphqlBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}
	return g
}

// GitHub has no REST endpoint that takes a pull request out of draft: the
// adapter reads the pull request's node ID, then posts the GraphQL mutation
// carrying it.
func TestMarkReadyPostsTheMutationWithThePullRequestsNodeID(t *testing.T) {
	m := &markReadyServer{
		nodeID:      "PR_kwDOAbc123",
		graphqlBody: `{"data":{"markPullRequestReadyForReview":{"pullRequest":{"isDraft":false}}}}`,
	}
	g := m.start(t)

	if err := g.MarkReady(context.Background(), "", "244"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	want := []string{"GET /repos/acme/api/pulls/244", "POST /graphql"}
	if !slices.Equal(m.requests, want) {
		t.Fatalf("requests = %v, want %v", m.requests, want)
	}
	for i, a := range m.auth {
		if a != "Bearer t" {
			t.Errorf("request %d Authorization = %q, want the adapter's token", i, a)
		}
	}
	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal([]byte(m.mutation), &payload); err != nil {
		t.Fatalf("GraphQL body is not JSON: %v: %s", err, m.mutation)
	}
	if !strings.Contains(payload.Query, "markPullRequestReadyForReview") {
		t.Errorf("query does not call markPullRequestReadyForReview: %s", payload.Query)
	}
	// The ID may be inlined in the query or passed as a variable; either way
	// it is the node ID read from the pull request, not its number.
	if !strings.Contains(m.mutation, `PR_kwDOAbc123`) {
		t.Errorf("mutation does not carry the pull request's node ID: %s", m.mutation)
	}
}

// GraphQL reports most failures with a 200 and an errors array; reading that
// as success would leave the pull request a draft while the stream believes it
// ready.
func TestMarkReadyReturnsGraphQLErrors(t *testing.T) {
	m := &markReadyServer{
		nodeID:      "PR_kwDOAbc123",
		graphqlBody: `{"data":{"markPullRequestReadyForReview":null},"errors":[{"type":"FORBIDDEN","message":"Resource not accessible by integration"}]}`,
	}
	g := m.start(t)

	err := g.MarkReady(context.Background(), "", "244")
	if err == nil {
		t.Fatal("a GraphQL response with errors was read as success")
	}
	if !strings.Contains(err.Error(), "Resource not accessible by integration") {
		t.Errorf("error does not name GraphQL's reason: %v", err)
	}
}

// A non-2xx from either call is an error, and a pull request that cannot be
// read is never marked.
func TestMarkReadyReturnsAnErrorOnAFailedCall(t *testing.T) {
	t.Run("graphql", func(t *testing.T) {
		m := &markReadyServer{nodeID: "PR_kwDOAbc123", graphqlStatus: http.StatusBadGateway, graphqlBody: `bad gateway`}
		g := m.start(t)
		if err := g.MarkReady(context.Background(), "", "244"); err == nil {
			t.Fatal("a 502 from the GraphQL endpoint was read as success")
		}
	})
	t.Run("pull request read", func(t *testing.T) {
		m := &markReadyServer{getStatus: http.StatusNotFound}
		g := m.start(t)
		if err := g.MarkReady(context.Background(), "", "244"); err == nil {
			t.Fatal("a 404 reading the pull request was read as success")
		}
		for _, r := range m.requests {
			if strings.HasPrefix(r, "POST ") {
				t.Errorf("posted %s after the pull request could not be read", r)
			}
		}
	})
}
