package versioncontrol

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FileAt reads one file's text at a ref through the contents API (Phase 40
// step 13): the delivery and discovery environments read main's roadmap with it.
func TestFileAtReturnsTheDecodedTextOfTheFileAtTheRef(t *testing.T) {
	const text = "## Phase 40 — Streams (Planned)\n\nbody with ünïcode\n"
	var method, path, ref string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, ref = r.Method, r.URL.Path, r.URL.Query().Get("ref")
		w.Header().Set("Content-Type", "application/json")
		// GitHub wraps the base64 at 60 columns, so the decoder must cope with newlines.
		enc := base64.StdEncoding.EncodeToString([]byte(text))
		wrapped := enc[:20] + "\n" + enc[20:]
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + strings.ReplaceAll(wrapped, "\n", `\n`) + `"}`))
	}))
	defer srv.Close()
	g := NewGitHub("t", "acme/api")
	g.client = &http.Client{Transport: toServer{srv}}

	got, err := g.FileAt(t.Context(), "", "main", "docs/roadmap.md")
	if err != nil {
		t.Fatalf("FileAt: %v", err)
	}
	if got != text {
		t.Errorf("FileAt = %q, want %q", got, text)
	}
	if method != http.MethodGet || path != "/repos/acme/api/contents/docs/roadmap.md" || ref != "main" {
		t.Errorf("asked %s %s?ref=%s, want GET /repos/acme/api/contents/docs/roadmap.md?ref=main", method, path, ref)
	}
}

func TestFileAtFailsOnANon2xx(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
		}))
		g := NewGitHub("t", "acme/api")
		g.client = &http.Client{Transport: toServer{srv}}
		got, err := g.FileAt(t.Context(), "", "main", "docs/roadmap.md")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: want an error, got text %q", status, got)
		}
	}
}

func TestNoOpFileAtIsUnsupported(t *testing.T) {
	got, err := NewNoOp().FileAt(t.Context(), "acme/api", "main", "docs/roadmap.md")
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
	if got != "" {
		t.Errorf("text = %q, want none", got)
	}
}
