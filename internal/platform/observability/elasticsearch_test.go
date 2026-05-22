package observability

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestElasticsearchExporter_InactiveSkipsHTTP(t *testing.T) {
	t.Setenv("ELASTICSEARCH_URL", "")
	e := NewElasticsearchExporter()
	if e.Active() {
		t.Fatalf("expected inactive when URL unset")
	}
	if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}}); err != nil {
		t.Errorf("inactive ExportMetrics should be no-op, got %v", err)
	}
}

func TestElasticsearchExporter_BulkFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/_bulk") {
			t.Errorf("expected /_bulk path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-ndjson" {
			t.Errorf("expected ndjson content-type, got %s", got)
		}
		body, _ := io.ReadAll(r.Body)
		// Verify the bulk format: alternating action/doc lines, newline-terminated.
		lines := bytes.Split(bytes.TrimRight(body, "\n"), []byte("\n"))
		if len(lines) != 4 { // 2 records → 2 action + 2 doc lines
			t.Errorf("expected 4 bulk lines for 2 records, got %d: %s", len(lines), body)
		}
		for i := 0; i < len(lines); i += 2 {
			if !bytes.Contains(lines[i], []byte(`"index"`)) {
				t.Errorf("expected action line %d to contain index, got %s", i, lines[i])
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":false,"items":[]}`))
	}))
	defer srv.Close()

	e := &ElasticsearchExporter{
		url: srv.URL, metricsIndex: "k-metrics", logsIndex: "k-logs",
		client: srv.Client(),
	}
	err := e.ExportMetrics(context.Background(), []MetricRecord{
		{Name: "a", Value: 1, Timestamp: time.Now()},
		{Name: "b", Value: 2, Timestamp: time.Now()},
	})
	if err != nil {
		t.Errorf("ExportMetrics: %v", err)
	}
}

func TestElasticsearchExporter_BasicAuth(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":false}`))
	}))
	defer srv.Close()

	e := &ElasticsearchExporter{
		url: srv.URL, username: "elastic", password: "secret",
		metricsIndex: "m", client: srv.Client(),
	}
	_ = e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}})
	if !strings.HasPrefix(capturedAuth, "Basic ") {
		t.Errorf("expected Basic auth, got %q", capturedAuth)
	}
}

func TestElasticsearchExporter_APIKeyOverridesBasic(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":false}`))
	}))
	defer srv.Close()

	e := &ElasticsearchExporter{
		url: srv.URL, username: "elastic", password: "secret", apiKey: "abc123",
		metricsIndex: "m", client: srv.Client(),
	}
	_ = e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}})
	if capturedAuth != "ApiKey abc123" {
		t.Errorf("expected ApiKey auth, got %q", capturedAuth)
	}
}
