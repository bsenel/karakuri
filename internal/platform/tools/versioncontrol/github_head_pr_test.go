package versioncontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// HasPullRequestForHead asks GitHub for the pull requests in every state whose
// head is the branch, so a merged or closed one counts (Phase 40 step 6). The
// head filter takes owner:branch; a bare branch name is ignored by GitHub and
// would list every pull request in the repository.
func TestHasPullRequestForHeadAsksForEveryStateOfThatBranch(t *testing.T) {
	const branch = "karakuri/phase-40-roadmap"
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"a merged pull request", `[{"number":169,"state":"closed","merged_at":"2026-11-01T10:00:00Z","head":{"ref":"` + branch + `"}}]`, true},
		{"no pull request", `[]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var method, path, state, head string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				state, head = r.URL.Query().Get("state"), r.URL.Query().Get("head")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			g := NewGitHub("t", "acme/api")
			g.client = &http.Client{Transport: toServer{srv}}

			got, err := g.HasPullRequestForHead(t.Context(), "", branch)
			if err != nil {
				t.Fatalf("HasPullRequestForHead: %v", err)
			}
			if method != http.MethodGet || path != "/repos/acme/api/pulls" {
				t.Errorf("request %s %s, want GET /repos/acme/api/pulls", method, path)
			}
			if state != "all" {
				t.Errorf("state = %q, want all: an open-only read misses a merged or closed pull request", state)
			}
			if head != "acme:"+branch {
				t.Errorf("head = %q, want %q", head, "acme:"+branch)
			}
			if got != c.want {
				t.Errorf("HasPullRequestForHead = %v for %s, want %v", got, c.body, c.want)
			}
		})
	}
}

// A forge that fails is an error, never a false: false would list a branch
// whose pull request was merged.
func TestHasPullRequestForHeadFailsOnAServerErrorForABranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"bad gateway"}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	if _, err := g.HasPullRequestForHead(t.Context(), "", "karakuri/phase-40-roadmap"); err == nil {
		t.Error("a 502 from the pulls endpoint returned no error")
	}
}
