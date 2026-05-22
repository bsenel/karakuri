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

func TestLokiExporter_InactiveSkipsHTTP(t *testing.T) {
	t.Setenv("LOKI_URL", "")
	e := NewLokiExporter()
	if e.Active() {
		t.Fatalf("expected inactive when URL unset")
	}
}

func TestLokiExporter_StreamFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/loki/api/v1/push") {
			t.Errorf("expected /loki/api/v1/push, got %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var p struct {
			Streams []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"streams"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Two distinct levels → two streams.
		if len(p.Streams) != 2 {
			t.Errorf("expected 2 streams (info + error), got %d: %s", len(p.Streams), body)
		}
		for _, s := range p.Streams {
			if _, ok := s.Stream["level"]; !ok {
				t.Errorf("stream missing level label: %+v", s.Stream)
			}
			if len(s.Values) == 0 {
				t.Errorf("stream has no values")
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	e := &LokiExporter{
		url: srv.URL, labels: map[string]string{"service": "karakuri"},
		client: srv.Client(),
	}
	err := e.ExportLogs(context.Background(), []LogRecord{
		{Level: "info", Message: "started", Timestamp: time.Now()},
		{Level: "info", Message: "ready", Timestamp: time.Now()},
		{Level: "error", Message: "boom", Timestamp: time.Now()},
	})
	if err != nil {
		t.Errorf("ExportLogs: %v", err)
	}
}

func TestLokiExporter_MetricsNoOp(t *testing.T) {
	e := &LokiExporter{url: "http://no-such-host"}
	// Should not error and should not make any network call.
	if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}}); err != nil {
		t.Errorf("ExportMetrics should be no-op, got %v", err)
	}
}

func TestLokiExporter_TenantHeader(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	e := &LokiExporter{url: srv.URL, tenantID: "acme", client: srv.Client(), labels: map[string]string{"service": "k"}}
	_ = e.ExportLogs(context.Background(), []LogRecord{{Level: "info", Message: "x", Timestamp: time.Now()}})
	if captured != "acme" {
		t.Errorf("expected X-Scope-OrgID=acme, got %q", captured)
	}
}
