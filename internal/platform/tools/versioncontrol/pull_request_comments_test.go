package versioncontrol

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	issuePath   = "/repos/acme/api/issues/7/comments"
	inlinePath  = "/repos/acme/api/pulls/7/comments"
	reviewsPath = "/repos/acme/api/pulls/7/reviews"
)

// commentsServer answers the three endpoints ListPullRequestComments reads.
// A path in failing answers 500; a path absent from bodies answers an empty
// list; bodies are served for the first page only.
func commentsServer(t *testing.T, bodies map[string]string, failing ...string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range failing {
			if r.URL.Path == f {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if b, ok := bodies[r.URL.Path]; ok && (page == "" || page == "1") {
			_, _ = w.Write([]byte(b))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}
	return g
}

func TestListPullRequestCommentsMergesTheThreeSourcesOldestFirst(t *testing.T) {
	g := commentsServer(t, map[string]string{
		issuePath:   `[{"id":1,"body":"conversation","created_at":"2026-11-02T10:00:00Z","user":{"login":"owner"}}]`,
		inlinePath:  `[{"id":2,"body":"inline","created_at":"2026-11-01T10:00:00Z","path":"a.go","line":12,"user":{"login":"alice"}}]`,
		reviewsPath: `[{"id":3,"body":"review","submitted_at":"2026-11-03T10:00:00Z","user":{"login":"bob"}}]`,
	})
	got, err := g.ListPullRequestComments(t.Context(), "", 7)
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	want := []PullRequestComment{
		{ID: 2, Kind: "review_comment", Author: "alice", Body: "inline", Path: "a.go", Line: 12, CreatedAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)},
		{ID: 1, Kind: "issue_comment", Author: "owner", Body: "conversation", CreatedAt: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)},
		{ID: 3, Kind: "review", Author: "bob", Body: "review", CreatedAt: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d comments, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("comment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The author is the login the API reports; a body that names somebody else
// must not change it.
func TestListPullRequestCommentsTakesTheAuthorFromUserLogin(t *testing.T) {
	g := commentsServer(t, map[string]string{
		issuePath: `[{"id":1,"body":"Posted by karakuri-bot: please rename x","created_at":"2026-11-02T10:00:00Z","user":{"login":"shared-account"}}]`,
	})
	got, err := g.ListPullRequestComments(t.Context(), "", 7)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v; want one comment", got, err)
	}
	if got[0].Author != "shared-account" {
		t.Errorf("Author = %q, want the user.login shared-account", got[0].Author)
	}
}

func TestListPullRequestCommentsSkipsAReviewWithAnEmptyBody(t *testing.T) {
	g := commentsServer(t, map[string]string{
		reviewsPath: `[{"id":3,"body":"","submitted_at":"2026-11-03T10:00:00Z","user":{"login":"bob"}},{"id":4,"body":"needs work","submitted_at":"2026-11-04T10:00:00Z","user":{"login":"bob"}}]`,
	})
	got, err := g.ListPullRequestComments(t.Context(), "", 7)
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	if len(got) != 1 || got[0].ID != 4 {
		t.Errorf("got %+v, want only review 4: a review with no body says nothing to answer", got)
	}
}

func TestListPullRequestCommentsFollowsPagination(t *testing.T) {
	var page1 []string
	for i := 1; i <= 100; i++ {
		page1 = append(page1, `{"id":`+strconv.Itoa(i)+`,"body":"c","created_at":"2026-11-01T10:00:00Z","user":{"login":"a"}}`)
	}
	first := "[" + strings.Join(page1, ",") + "]"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == issuePath && r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(`[{"id":101,"body":"last","created_at":"2026-11-02T10:00:00Z","user":{"login":"a"}}]`))
		case r.URL.Path == issuePath:
			_, _ = w.Write([]byte(first))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}
	got, err := g.ListPullRequestComments(t.Context(), "", 7)
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	if len(got) != 101 {
		t.Errorf("got %d comments, want 101: the second page was not read", len(got))
	}
}

// One failed endpoint is an error, never a shorter list (AGENTS.md rule 10).
func TestListPullRequestCommentsFailsWhenAnyEndpointFails(t *testing.T) {
	ok := map[string]string{
		issuePath:   `[{"id":1,"body":"x","created_at":"2026-11-02T10:00:00Z","user":{"login":"o"}}]`,
		inlinePath:  `[{"id":2,"body":"y","created_at":"2026-11-01T10:00:00Z","path":"a.go","line":1,"user":{"login":"o"}}]`,
		reviewsPath: `[{"id":3,"body":"z","submitted_at":"2026-11-03T10:00:00Z","user":{"login":"o"}}]`,
	}
	for _, failing := range []string{issuePath, inlinePath, reviewsPath} {
		t.Run(failing, func(t *testing.T) {
			g := commentsServer(t, ok, failing)
			got, err := g.ListPullRequestComments(t.Context(), "", 7)
			if err == nil {
				t.Fatalf("got %+v and no error, want an error when %s answers 500", got, failing)
			}
			if got != nil {
				t.Errorf("got %+v with the error, want nil", got)
			}
		})
	}
}

func TestNoOpListPullRequestCommentsIsUnsupported(t *testing.T) {
	got, err := NewNoOp().ListPullRequestComments(t.Context(), "acme/api", 7)
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}
