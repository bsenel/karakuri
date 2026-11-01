package versioncontrol

import (
	"context"
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
