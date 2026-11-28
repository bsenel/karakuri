package versioncontrol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Phase 40 step 11: the reviewer posts one review that only comments. Where
// the author and the reviewer are one account, approval stays the owner's
// merge, so the event is COMMENT and never anything else.
func TestPostReviewSendsOneCommentReview(t *testing.T) {
	var method, path string
	var sent map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		method, path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sent)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	comments := []ReviewComment{
		{Path: "cli/main.go", Line: 42, Body: "`krk bogus` exited 0"},
		{Path: "internal/x.go", Line: 7, Body: "does not build"},
	}
	if err := g.PostReview(context.Background(), "", "244", "What was run: go build ./...", comments); err != nil {
		t.Fatalf("PostReview: %v", err)
	}
	if calls != 1 {
		t.Errorf("forge was called %d times, want one review", calls)
	}
	if method != "POST" || path != "/repos/acme/api/pulls/244/reviews" {
		t.Errorf("sent %s %s, want POST /repos/acme/api/pulls/244/reviews", method, path)
	}
	if sent["event"] != "COMMENT" {
		t.Errorf("event = %v, want COMMENT", sent["event"])
	}
	if sent["body"] != "What was run: go build ./..." {
		t.Errorf("body = %v, want the given body", sent["body"])
	}
	var got []ReviewComment
	raw, _ := json.Marshal(sent["comments"])
	var wire []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("comments %v are not a list of path, line and body: %v", sent["comments"], err)
	}
	for _, c := range wire {
		got = append(got, ReviewComment{Path: c.Path, Line: c.Line, Body: c.Body})
	}
	if !reflect.DeepEqual(got, comments) {
		t.Errorf("comments = %+v, want %+v", got, comments)
	}
}

// A review with no line comments is still a COMMENT review with its body.
func TestPostReviewWithoutLineCommentsStillComments(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sent)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	if err := g.PostReview(context.Background(), "acme/other", "9", "clean", nil); err != nil {
		t.Fatalf("PostReview: %v", err)
	}
	if sent["event"] != "COMMENT" || sent["body"] != "clean" {
		t.Errorf("sent %v, want a COMMENT review with the body", sent)
	}
}

// A review the forge refused was not posted, and saying so is the only honest
// answer.
func TestPostReviewReturnsAnErrorOnANon2xx(t *testing.T) {
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"no"}`))
			}))
			defer srv.Close()
			g := NewGitHub("t", "acme/api")
			g.client = &http.Client{Transport: toServer{srv}}

			err := g.PostReview(context.Background(), "", "244", "body", nil)
			if err == nil {
				t.Fatalf("a %d was read as success", status)
			}
			if !strings.HasPrefix(err.Error(), "github") {
				t.Errorf("error %q does not name the adapter", err)
			}
		})
	}
}

// With no forge nothing was posted, and a nil would let the stream believe a
// review exists (AGENTS.md rule 10).
func TestNoOpPostReviewIsUnsupported(t *testing.T) {
	err := NewNoOp().PostReview(context.Background(), "acme/api", "244", "body", nil)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PostReview = %v, want an error wrapping ErrUnsupported", err)
	}
	if !strings.HasPrefix(err.Error(), "noop") {
		t.Errorf("error %q does not name the adapter", err)
	}
}

// Phase 40 step 11: a pull request whose head has no review is found by the
// commit ids its reviews were left at.
func TestReviewedCommitsListsTheCommitOfEveryReview(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`[{"id":1,"commit_id":"aaa111"},{"id":2,"commit_id":"bbb222"}]`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	got, err := g.ReviewedCommits(context.Background(), "", "244")
	if err != nil {
		t.Fatalf("ReviewedCommits: %v", err)
	}
	if method != "GET" || path != "/repos/acme/api/pulls/244/reviews" {
		t.Errorf("sent %s %s, want GET /repos/acme/api/pulls/244/reviews", method, path)
	}
	if !reflect.DeepEqual(got, []string{"aaa111", "bbb222"}) {
		t.Errorf("commits = %v, want [aaa111 bbb222]", got)
	}
}

// Phase 40 step 14: the digest shows the latest review's result, so the state
// of each review is read along with its commit.
func TestListReviewsReadsTheStateAndCommitOfEveryReview(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`[{"id":1,"state":"CHANGES_REQUESTED","commit_id":"aaa111"},{"id":2,"state":"APPROVED","commit_id":"bbb222"}]`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	got, err := g.ListReviews(context.Background(), "", "244")
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if path != "/repos/acme/api/pulls/244/reviews" {
		t.Errorf("sent %s, want /repos/acme/api/pulls/244/reviews", path)
	}
	want := []Review{{State: "changes_requested", CommitID: "aaa111"}, {State: "approved", CommitID: "bbb222"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reviews = %v, want %v", got, want)
	}
}

func TestNoOpListReviewsIsUnsupported(t *testing.T) {
	got, err := NewNoOp().ListReviews(context.Background(), "acme/api", "1")
	if !errors.Is(err, ErrUnsupported) || got != nil {
		t.Errorf("got %v, %v; want nil and ErrUnsupported", got, err)
	}
}

// A refused read is an error, never an empty list: no reviews and unreadable
// reviews are different answers.
func TestReviewedCommitsNon2xxIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	got, err := g.ReviewedCommits(context.Background(), "", "244")
	if err == nil {
		t.Fatalf("ReviewedCommits = %v with no error on a 403, want an error", got)
	}
	if strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error %q is the scaffold's, not the forge's refusal", err)
	}
}

// AGENTS.md rule 10: an adapter without a backend says so.
func TestNoOpReviewedCommitsIsUnsupported(t *testing.T) {
	got, err := NewNoOp().ReviewedCommits(context.Background(), "acme/api", "1")
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
	if got != nil {
		t.Errorf("commits = %v, want none alongside the error", got)
	}
}
