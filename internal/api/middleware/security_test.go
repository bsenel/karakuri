package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/api/middleware"
)

func TestSecurityHeadersSet(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors: %q", csp)
	}
}

func TestMaxBytesRejectsOversizedBody(t *testing.T) {
	h := middleware.MaxBytes(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	body := strings.NewReader(strings.Repeat("a", 64))
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/x", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader("ok")))
	if rec2.Code != http.StatusOK {
		t.Errorf("small body: status = %d, want %d", rec2.Code, http.StatusOK)
	}
}
