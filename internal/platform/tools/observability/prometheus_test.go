package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var _ ObservabilityAdapter = (*Prometheus)(nil)

// promAlert is one entry of Prometheus' /api/v1/alerts response.
type promAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt"`
	Value       string            `json:"value"`
}

var promT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func firingAlert(name, service, severity string, activeAt time.Time) promAlert {
	labels := map[string]string{"alertname": name}
	if service != "" {
		labels["service"] = service
	}
	if severity != "" {
		labels["severity"] = severity
	}
	return promAlert{
		Labels:      labels,
		Annotations: map[string]string{"summary": name + " summary"},
		State:       "firing",
		ActiveAt:    activeAt.Format(time.RFC3339),
		Value:       "1e+00",
	}
}

func alertsBody(t *testing.T, alerts ...promAlert) string {
	t.Helper()
	if alerts == nil {
		alerts = []promAlert{}
	}
	b, err := json.Marshal(map[string]any{
		"status": "success",
		"data":   map[string]any{"alerts": alerts},
	})
	if err != nil {
		t.Fatalf("marshal alerts: %v", err)
	}
	return string(b)
}

// promServer serves one canned response and records every request it receives.
type promServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newPromServer(t *testing.T, status int, body string) *promServer {
	t.Helper()
	ps := &promServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		ps.requests = append(ps.requests, r.Clone(context.Background()))
		ps.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ps.Close)
	return ps
}

func (ps *promServer) only(t *testing.T) *http.Request {
	t.Helper()
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(ps.requests))
	}
	return ps.requests[0]
}

func (ps *promServer) count() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.requests)
}

func getAlerts(t *testing.T, service string, since time.Time, threshold string, alerts ...promAlert) []Alert {
	t.Helper()
	ps := newPromServer(t, http.StatusOK, alertsBody(t, alerts...))
	got, err := NewPrometheus(ps.URL, "").GetAlerts(context.Background(), "prod", service, since, threshold)
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	return got
}

// assertNames compares the alerts' names, which firingAlert carries in the
// summary annotation and so in Message.
func assertNames(t *testing.T, got []Alert, want ...string) {
	t.Helper()
	names := make([]string, 0, len(got))
	for _, a := range got {
		names = append(names, strings.TrimSuffix(a.Message, " summary"))
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("alerts = %v, want %v", names, want)
	}
}

func TestPrometheus_Name(t *testing.T) {
	if got := NewPrometheus("http://prom", "").Name(); got != "prometheus" {
		t.Errorf("Name() = %q, want prometheus", got)
	}
}

func TestPrometheus_Active(t *testing.T) {
	if NewPrometheus("", "").Active() {
		t.Errorf("Active() with empty url should be false")
	}
	if NewPrometheus("", "tok").Active() {
		t.Errorf("Active() with a token but empty url should be false")
	}
	if !NewPrometheus("http://prom:9090", "").Active() {
		t.Errorf("Active() with a url should be true")
	}
}

func TestPrometheus_GetAlerts_Request(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, alertsBody(t))
	if _, err := NewPrometheus(ps.URL, "s3cret").GetAlerts(context.Background(), "prod", "", time.Time{}, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	r := ps.only(t)
	if r.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != "/api/v1/alerts" {
		t.Errorf("path = %s, want /api/v1/alerts", r.URL.Path)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer s3cret")
	}
}

func TestPrometheus_GetAlerts_NoTokenSendsNoAuthorization(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, alertsBody(t))
	if _, err := NewPrometheus(ps.URL, "").GetAlerts(context.Background(), "prod", "", time.Time{}, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	if v, ok := ps.only(t).Header["Authorization"]; ok {
		t.Errorf("Authorization header should be absent without a token, got %q", v)
	}
}

func TestPrometheus_GetAlerts_Mapping(t *testing.T) {
	got := getAlerts(t, "", time.Time{}, "", promAlert{
		Labels:      map[string]string{"alertname": "HighErrorRate", "service": "checkout", "job": "checkout-job", "severity": "critical"},
		Annotations: map[string]string{"summary": "5xx above 5%", "description": "long text"},
		State:       "firing",
		ActiveAt:    promT0.Format(time.RFC3339),
		Value:       "1e+00",
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(got))
	}
	a := got[0]
	if a.State != AlertFiring {
		t.Errorf("State = %q, want %q", a.State, AlertFiring)
	}
	if a.Service != "checkout" {
		t.Errorf("Service = %q, want checkout (service label wins over job)", a.Service)
	}
	if a.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", a.Severity)
	}
	if a.Message != "5xx above 5%" {
		t.Errorf("Message = %q, want the summary annotation", a.Message)
	}
	if !a.Time.Equal(promT0) {
		t.Errorf("Time = %v, want %v", a.Time, promT0)
	}
	if a.ID == "" {
		t.Errorf("ID should not be empty")
	}
}

func TestPrometheus_GetAlerts_ServiceFallsBackToJob(t *testing.T) {
	got := getAlerts(t, "", time.Time{}, "", promAlert{
		Labels:   map[string]string{"alertname": "Down", "job": "api"},
		State:    "firing",
		ActiveAt: promT0.Format(time.RFC3339),
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(got))
	}
	if got[0].Service != "api" {
		t.Errorf("Service = %q, want api from the job label", got[0].Service)
	}
}

func TestPrometheus_GetAlerts_MessageFallbacks(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{"summary", map[string]string{"summary": "sum", "description": "desc"}, "sum"},
		{"description", map[string]string{"description": "desc"}, "desc"},
		{"alertname", nil, "Down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getAlerts(t, "", time.Time{}, "", promAlert{
				Labels:      map[string]string{"alertname": "Down", "job": "api"},
				Annotations: tc.annotations,
				State:       "firing",
				ActiveAt:    promT0.Format(time.RFC3339),
			})
			if len(got) != 1 {
				t.Fatalf("expected 1 alert, got %d", len(got))
			}
			if got[0].Message != tc.want {
				t.Errorf("Message = %q, want %q", got[0].Message, tc.want)
			}
		})
	}
}

func TestPrometheus_GetAlerts_SkipsPendingAndInactive(t *testing.T) {
	pending := firingAlert("Pending", "api", "warning", promT0)
	pending.State = "pending"
	inactive := firingAlert("Inactive", "api", "warning", promT0)
	inactive.State = "inactive"
	got := getAlerts(t, "", time.Time{}, "", pending, firingAlert("Firing", "api", "warning", promT0), inactive)
	assertNames(t, got, "Firing")
	for _, a := range got {
		if a.State != AlertFiring {
			t.Errorf("State = %q, want %q", a.State, AlertFiring)
		}
	}
}

func TestPrometheus_GetAlerts_IDIsStable(t *testing.T) {
	first := firingAlert("HighErrorRate", "checkout", "critical", promT0)
	later := firingAlert("HighErrorRate", "checkout", "critical", promT0.Add(3*time.Hour))
	later.Value = "4.2e+01"
	otherLabel := firingAlert("HighErrorRate", "payments", "critical", promT0)

	a := getAlerts(t, "", time.Time{}, "", first)
	b := getAlerts(t, "", time.Time{}, "", later)
	c := getAlerts(t, "", time.Time{}, "", otherLabel)
	if len(a) != 1 || len(b) != 1 || len(c) != 1 {
		t.Fatalf("expected 1 alert per call, got %d, %d, %d", len(a), len(b), len(c))
	}
	if a[0].ID == "" {
		t.Fatalf("ID should not be empty")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a[0].ID) {
		t.Errorf("ID = %q, want lowercase hex sha256", a[0].ID)
	}
	if a[0].ID != b[0].ID {
		t.Errorf("ID changed with activeAt/value: %q vs %q", a[0].ID, b[0].ID)
	}
	if a[0].ID == c[0].ID {
		t.Errorf("ID should differ when a label differs, both %q", a[0].ID)
	}
}

func TestPrometheus_GetAlerts_ServiceFilter(t *testing.T) {
	alerts := []promAlert{
		firingAlert("A", "checkout", "warning", promT0),
		firingAlert("B", "payments", "warning", promT0),
		{
			Labels:      map[string]string{"alertname": "C", "job": "checkout"},
			Annotations: map[string]string{"summary": "C summary"},
			State:       "firing",
			ActiveAt:    promT0.Format(time.RFC3339),
		},
	}
	assertNames(t, getAlerts(t, "checkout", time.Time{}, "", alerts...), "A", "C")
	assertNames(t, getAlerts(t, "", time.Time{}, "", alerts...), "A", "B", "C")
}

func TestPrometheus_GetAlerts_SinceFilter(t *testing.T) {
	alerts := []promAlert{
		firingAlert("Old", "api", "warning", promT0.Add(-2*time.Hour)),
		firingAlert("New", "api", "warning", promT0.Add(time.Hour)),
	}
	assertNames(t, getAlerts(t, "", promT0, "", alerts...), "New")
	assertNames(t, getAlerts(t, "", time.Time{}, "", alerts...), "New", "Old")
}

func TestPrometheus_GetAlerts_ThresholdFilter(t *testing.T) {
	alerts := []promAlert{
		firingAlert("Info", "api", "info", promT0),
		firingAlert("Warning", "api", "warning", promT0),
		firingAlert("Error", "api", "error", promT0),
		firingAlert("Critical", "api", "CRITICAL", promT0),
		firingAlert("Missing", "api", "", promT0),
		firingAlert("Unranked", "api", "page", promT0),
	}
	cases := []struct {
		threshold string
		want      []string
	}{
		{"", []string{"Critical", "Error", "Info", "Missing", "Unranked", "Warning"}},
		{"info", []string{"Critical", "Error", "Info", "Missing", "Unranked", "Warning"}},
		{"warning", []string{"Critical", "Error", "Missing", "Unranked", "Warning"}},
		{"error", []string{"Critical", "Error", "Missing", "Unranked"}},
		{"critical", []string{"Critical", "Missing", "Unranked"}},
		{"Error", []string{"Critical", "Error", "Missing", "Unranked"}},
	}
	for _, tc := range cases {
		t.Run("threshold="+tc.threshold, func(t *testing.T) {
			assertNames(t, getAlerts(t, "", time.Time{}, tc.threshold, alerts...), tc.want...)
		})
	}
}

func TestPrometheus_GetAlerts_UnknownThresholdIsError(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, alertsBody(t, firingAlert("A", "api", "critical", promT0)))
	got, err := NewPrometheus(ps.URL, "").GetAlerts(context.Background(), "prod", "", time.Time{}, "loud")
	if err == nil {
		t.Fatalf("expected an error for an unknown threshold, got %d alerts", len(got))
	}
	if !strings.Contains(err.Error(), "loud") {
		t.Errorf("error %q should name the bad threshold", err)
	}
}

const matrixBody = `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up","job":"api"},"values":[[1700000000,"1"],[1700000060,"0.5"]]}]}}`

func TestPrometheus_FetchMetrics_Request(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, matrixBody)
	since := time.Unix(1700000000, 0)
	_, err := NewPrometheus(ps.URL, "s3cret").FetchMetrics(context.Background(), MetricQuery{
		Query: `up{job="api"}`,
		Since: since,
		Step:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("FetchMetrics: %v", err)
	}
	r := ps.only(t)
	if r.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != "/api/v1/query_range" {
		t.Errorf("path = %s, want /api/v1/query_range", r.URL.Path)
	}
	q := r.URL.Query()
	if got := q.Get("query"); got != `up{job="api"}` {
		t.Errorf("query = %q", got)
	}
	start, err := strconv.ParseFloat(q.Get("start"), 64)
	if err != nil {
		t.Fatalf("start %q does not parse: %v", q.Get("start"), err)
	}
	if int64(start) != since.Unix() {
		t.Errorf("start = %v, want %d", start, since.Unix())
	}
	end, err := strconv.ParseFloat(q.Get("end"), 64)
	if err != nil {
		t.Fatalf("end %q does not parse: %v", q.Get("end"), err)
	}
	if end < start {
		t.Errorf("end %v is before start %v", end, start)
	}
	if now := float64(time.Now().Unix()); end < now-60 || end > now+60 {
		t.Errorf("end = %v, want about now (%v)", end, now)
	}
	if step, err := strconv.ParseFloat(q.Get("step"), 64); err != nil || step != 30 {
		t.Errorf("step = %q, want 30 seconds", q.Get("step"))
	}
	if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer s3cret")
	}
}

func TestPrometheus_FetchMetrics_DefaultStepAndNoToken(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, matrixBody)
	if _, err := NewPrometheus(ps.URL, "").FetchMetrics(context.Background(), MetricQuery{Query: "up", Since: time.Unix(1700000000, 0)}); err != nil {
		t.Fatalf("FetchMetrics: %v", err)
	}
	r := ps.only(t)
	if step, err := strconv.ParseFloat(r.URL.Query().Get("step"), 64); err != nil || step != 60 {
		t.Errorf("step = %q, want 60 when Step is zero", r.URL.Query().Get("step"))
	}
	if v, ok := r.Header["Authorization"]; ok {
		t.Errorf("Authorization header should be absent without a token, got %q", v)
	}
}

func TestPrometheus_FetchMetrics_EmptyQueryMakesNoRequest(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, matrixBody)
	got, err := NewPrometheus(ps.URL, "").FetchMetrics(context.Background(), MetricQuery{Since: time.Unix(1700000000, 0)})
	if err == nil {
		t.Fatalf("expected an error for an empty query, got %d series", len(got))
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("error %q should say the query is missing", err)
	}
	if n := ps.count(); n != 0 {
		t.Errorf("empty query made %d requests, want 0", n)
	}
}

func TestPrometheus_FetchMetrics_Mapping(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, matrixBody)
	got, err := NewPrometheus(ps.URL, "").FetchMetrics(context.Background(), MetricQuery{Query: "up", Since: time.Unix(1700000000, 0)})
	if err != nil {
		t.Fatalf("FetchMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 series, got %d", len(got))
	}
	s := got[0]
	if s.Name != "up" {
		t.Errorf("Name = %q, want up", s.Name)
	}
	if len(s.Labels) != 1 || s.Labels["job"] != "api" {
		t.Errorf("Labels = %v, want only job=api", s.Labels)
	}
	if len(s.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(s.Points))
	}
	want := []MetricPoint{
		{Time: time.Unix(1700000000, 0), Value: 1},
		{Time: time.Unix(1700000060, 0), Value: 0.5},
	}
	for i, p := range s.Points {
		if !p.Time.Equal(want[i].Time) || p.Value != want[i].Value {
			t.Errorf("point %d = {%v %v}, want {%v %v}", i, p.Time, p.Value, want[i].Time, want[i].Value)
		}
	}
}

func TestPrometheus_FetchLogs_Unsupported(t *testing.T) {
	ps := newPromServer(t, http.StatusOK, `{"status":"success","data":{}}`)
	lines, err := NewPrometheus(ps.URL, "").FetchLogs(context.Background(), LogQuery{Service: "api", Query: "error", Limit: 10})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want errors.Is(err, ErrUnsupported)", err)
	}
	if !strings.Contains(err.Error(), "prometheus") {
		t.Errorf("error %q should name prometheus", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected no log lines, got %d", len(lines))
	}
	if n := ps.count(); n != 0 {
		t.Errorf("FetchLogs made %d requests, want 0", n)
	}
}

// promCalls runs both HTTP-backed calls against a server, so each failure mode
// is pinned for alerts and for metrics.
var promCalls = []struct {
	name string
	call func(p *Prometheus) (int, error)
}{
	{"GetAlerts", func(p *Prometheus) (int, error) {
		got, err := p.GetAlerts(context.Background(), "prod", "", time.Time{}, "")
		return len(got), err
	}},
	{"FetchMetrics", func(p *Prometheus) (int, error) {
		got, err := p.FetchMetrics(context.Background(), MetricQuery{Query: "up", Since: time.Unix(1700000000, 0)})
		return len(got), err
	}},
}

func TestPrometheus_NonSuccessStatusIsError(t *testing.T) {
	for _, tc := range promCalls {
		t.Run(tc.name, func(t *testing.T) {
			ps := newPromServer(t, http.StatusServiceUnavailable, `upstream unavailable`)
			n, err := tc.call(NewPrometheus(ps.URL, ""))
			if err == nil {
				t.Fatalf("expected an error for a 503, got %d results", n)
			}
			if ps.count() != 1 {
				t.Fatalf("expected the call to reach the server once, got %d requests", ps.count())
			}
			if !strings.Contains(err.Error(), "503") {
				t.Errorf("error %q should include the status code", err)
			}
			if n != 0 {
				t.Errorf("expected no results alongside the error, got %d", n)
			}
		})
	}
}

func TestPrometheus_ErrorStatusBodyIsError(t *testing.T) {
	const body = `{"status":"error","errorType":"bad_data","error":"invalid parameter \"query\""}`
	for _, tc := range promCalls {
		t.Run(tc.name, func(t *testing.T) {
			ps := newPromServer(t, http.StatusOK, body)
			n, err := tc.call(NewPrometheus(ps.URL, ""))
			if err == nil {
				t.Fatalf("expected an error for status=error, got %d results", n)
			}
			if ps.count() != 1 {
				t.Fatalf("expected the call to reach the server once, got %d requests", ps.count())
			}
			for _, want := range []string{"error", "bad_data", "invalid parameter"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should include %q", err, want)
				}
			}
			if n != 0 {
				t.Errorf("expected no results alongside the error, got %d", n)
			}
		})
	}
}

func TestPrometheus_OversizedBodyIsError(t *testing.T) {
	// Valid JSON on purpose: an adapter that reads without a cap would decode
	// it and succeed.
	const pad = 9 << 20
	bodies := map[string][2]string{
		"GetAlerts":    {`{"status":"success","data":{"alerts":[],"pad":"`, `"}}`},
		"FetchMetrics": {`{"status":"success","data":{"resultType":"matrix","result":[],"pad":"`, `"}}`},
	}
	chunk := []byte(strings.Repeat("a", 64<<10))
	for _, tc := range promCalls {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			parts := bodies[tc.name]
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, parts[0])
				for written := 0; written < pad; written += len(chunk) {
					if _, err := w.Write(chunk); err != nil {
						return
					}
				}
				_, _ = fmt.Fprint(w, parts[1])
			}))
			t.Cleanup(srv.Close)

			n, err := tc.call(NewPrometheus(srv.URL, ""))
			if err == nil {
				t.Fatalf("expected an error for a body over 8 MiB, got %d results", n)
			}
			if hits.Load() != 1 {
				t.Fatalf("expected the call to reach the server once, got %d requests", hits.Load())
			}
		})
	}
}
