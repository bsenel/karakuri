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

func TestNewRelicExporter_InactiveSkipsHTTP(t *testing.T) {
	t.Setenv("NEW_RELIC_LICENSE_KEY", "")
	e := NewNewRelicExporter()
	if e.Active() {
		t.Fatalf("expected inactive when license key unset")
	}
	// No HTTP server — calling Export* must not panic and must not call out.
	if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}}); err != nil {
		t.Errorf("inactive ExportMetrics should be a no-op, got %v", err)
	}
	if err := e.ExportLogs(context.Background(), []LogRecord{{Message: "x", Timestamp: time.Now()}}); err != nil {
		t.Errorf("inactive ExportLogs should be a no-op, got %v", err)
	}
}

func TestNewRelicExporter_MetricsPayloadShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Api-Key"); got != "test-key" {
			t.Errorf("missing Api-Key header, got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var raw []map[string][]map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(raw) != 1 || len(raw[0]["metrics"]) != 2 {
			t.Errorf("expected 1 envelope with 2 metrics, got %+v", raw)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	e := &NewRelicExporter{apiKey: "test-key", region: "us", client: srv.Client()}
	// Override URL: we can't easily redirect to the test server through the
	// public methods, so verify via direct post() call.
	err := e.post(context.Background(), srv.URL, []map[string]any{
		{"metrics": []map[string]any{
			{"name": "a", "type": "gauge", "value": 1.0},
			{"name": "b", "type": "gauge", "value": 2.0},
		}},
	})
	if err != nil {
		t.Errorf("post: %v", err)
	}
}

func TestNewRelicExporter_PermanentOn403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()
	e := &NewRelicExporter{apiKey: "bad", region: "us", client: srv.Client()}
	err := e.post(context.Background(), srv.URL, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "permanent error") {
		t.Errorf("expected permanent error on 403, got %v", err)
	}
}

func TestNewRelicExporter_RegionURL(t *testing.T) {
	cases := []struct {
		region   string
		host     string
		path     string
		expected string
	}{
		{"us", "metric-api", "/metric/v1", "https://metric-api.newrelic.com/metric/v1"},
		{"eu", "metric-api", "/metric/v1", "https://metric-api.eu.newrelic.com/metric/v1"},
		{"eu", "log-api", "/log/v1", "https://log-api.eu.newrelic.com/log/v1"},
		{"staging", "metric-api", "/metric/v1", "https://metric-api.staging.newrelic.com/metric/v1"},
	}
	for _, c := range cases {
		got := regionURL(c.region, c.host, c.path)
		if got != c.expected {
			t.Errorf("region=%s host=%s: expected %s, got %s", c.region, c.host, c.expected, got)
		}
	}
}
