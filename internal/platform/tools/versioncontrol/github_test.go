package versioncontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
