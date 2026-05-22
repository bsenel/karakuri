package observability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOTLPExporter_InactiveSkipsHTTP(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	e := NewOTLPExporter()
	if e.Active() {
		t.Fatalf("expected inactive when endpoint unset")
	}
}

func TestOTLPExporter_MetricsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/metrics") {
			t.Errorf("expected /v1/metrics, got %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var p struct {
			ResourceMetrics []struct {
				ScopeMetrics []struct {
					Metrics []map[string]any `json:"metrics"`
				} `json:"scopeMetrics"`
			} `json:"resourceMetrics"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(p.ResourceMetrics) == 0 || len(p.ResourceMetrics[0].ScopeMetrics) == 0 {
			t.Errorf("missing resourceMetrics/scopeMetrics: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := &OTLPExporter{endpoint: srv.URL, service: "karakuri", client: srv.Client()}
	_ = e.ExportMetrics(context.Background(), []MetricRecord{
		{Name: "a", Value: 1, Timestamp: time.Now()},
		{Name: "b", Value: 2, Timestamp: time.Now()},
	})
}

func TestOTLPExporter_LogsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/logs") {
			t.Errorf("expected /v1/logs, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := &OTLPExporter{endpoint: srv.URL, service: "karakuri", client: srv.Client()}
	if err := e.ExportLogs(context.Background(), []LogRecord{{Level: "info", Message: "x", Timestamp: time.Now()}}); err != nil {
		t.Errorf("ExportLogs: %v", err)
	}
}

func TestOTLPExporter_CustomHeaders(t *testing.T) {
	var captured map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = map[string]string{
			"Authorization": r.Header.Get("Authorization"),
			"X-Tenant":      r.Header.Get("X-Tenant"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := &OTLPExporter{
		endpoint: srv.URL,
		headers:  map[string]string{"Authorization": "Bearer abc", "X-Tenant": "acme"},
		service:  "karakuri",
		client:   srv.Client(),
	}
	_ = e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}})
	if captured["Authorization"] != "Bearer abc" || captured["X-Tenant"] != "acme" {
		t.Errorf("expected custom headers, got %+v", captured)
	}
}

func TestSeverityNumber(t *testing.T) {
	cases := map[string]int{
		"trace": 2, "debug": 6, "info": 10, "": 10, "warn": 14, "error": 18, "fatal": 22, "unknown": 10,
	}
	for in, want := range cases {
		if got := severityNumber(in); got != want {
			t.Errorf("severityNumber(%q): expected %d, got %d", in, want, got)
		}
	}
}
