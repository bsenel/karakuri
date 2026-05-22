package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusExporter_ScrapeFormat(t *testing.T) {
	e := NewPrometheusExporter()
	now := time.Unix(1700000000, 0)
	_ = e.ExportMetrics(context.Background(), []MetricRecord{
		{Name: "loop_iterations", Value: 5, Labels: map[string]string{"domain": "software"}, Timestamp: now},
		{Name: "loop_iterations", Value: 3, Labels: map[string]string{"domain": "agriculture"}, Timestamp: now},
		{Name: "agent_invocations", Value: 12, Timestamp: now},
	})

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rr.Body.String()

	for _, want := range []string{
		"# TYPE loop_iterations gauge",
		`loop_iterations{domain="software"} 5`,
		`loop_iterations{domain="agriculture"} 3`,
		"agent_invocations 12",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape body missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestPrometheusExporter_LatestValueWins(t *testing.T) {
	e := NewPrometheusExporter()
	now := time.Now()
	_ = e.ExportMetrics(context.Background(), []MetricRecord{
		{Name: "x", Value: 1, Labels: map[string]string{"a": "b"}, Timestamp: now},
		{Name: "x", Value: 5, Labels: map[string]string{"a": "b"}, Timestamp: now.Add(time.Second)},
	})
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `x{a="b"} 5`) {
		t.Errorf("expected latest value (5) to win, body=%s", body)
	}
	if strings.Contains(body, `x{a="b"} 1`) {
		t.Errorf("old value (1) should have been overwritten")
	}
}

func TestPrometheusExporter_LogsNoOp(t *testing.T) {
	e := NewPrometheusExporter()
	if err := e.ExportLogs(context.Background(), []LogRecord{{Level: "info", Message: "x", Timestamp: time.Now()}}); err != nil {
		t.Errorf("ExportLogs should be no-op, got %v", err)
	}
}

func TestPrometheusExporter_Pushgateway(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/metrics/job/karakuri") {
			t.Errorf("expected pushgateway path /metrics/job/karakuri, got %s", r.URL.Path)
		}
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		captured = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := &PrometheusExporter{
		pushURL: srv.URL, jobName: "karakuri",
		client: srv.Client(),
		series: make(map[string]*promSeries),
	}
	_ = e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now()}})
	if !strings.Contains(captured, "x 1") {
		t.Errorf("pushgateway payload missing metric, got %q", captured)
	}
}

func TestEscapeLabelValue(t *testing.T) {
	cases := map[string]string{
		`simple`:           `simple`,
		`with "quote"`:     `with \"quote\"`,
		`back\slash`:       `back\\slash`,
		"multi\nline":      `multi\nline`,
	}
	for in, want := range cases {
		if got := escapeLabelValue(in); got != want {
			t.Errorf("escapeLabelValue(%q): expected %q, got %q", in, want, got)
		}
	}
}
