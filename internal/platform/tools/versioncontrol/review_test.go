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
