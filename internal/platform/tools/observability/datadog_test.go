package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var _ ObservabilityAdapter = (*Datadog)(nil)

var ddT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// ddMonitor is one entry of Datadog's /api/v1/monitor response.
type ddMonitor struct {
	ID                   int64    `json:"id"`
	Name                 string   `json:"name"`
	OverallState         string   `json:"overall_state"`
	Tags                 []string `json:"tags"`
	OverallStateModified string   `json:"overall_state_modified,omitempty"`
	Modified             string   `json:"modified,omitempty"`
}

// ddMon is a monitor in the given state whose overall_state_modified is at.
func ddMon(id int64, name, state string, at time.Time, tags ...string) ddMonitor {
	return ddMonitor{
		ID:                   id,
		Name:                 name,
		OverallState:         state,
		Tags:                 tags,
		OverallStateModified: at.Format(time.RFC3339),
		Modified:             at.Add(-24 * time.Hour).Format(time.RFC3339),
	}
}

func monitorsBody(t *testing.T, monitors ...ddMonitor) string {
	t.Helper()
	if monitors == nil {
		monitors = []ddMonitor{}
	}
	b, err := json.Marshal(monitors)
	if err != nil {
		t.Fatalf("marshal monitors: %v", err)
	}
	return string(b)
}

// ddRequest is what ddServer keeps of a request, the body included.
type ddRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// ddServer serves one canned response and records every request it receives.
type ddServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []ddRequest
}

func newDDServer(t *testing.T, status int, body string) *ddServer {
	t.Helper()
	ds := &ddServer{}
	ds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBody, _ := io.ReadAll(r.Body)
		ds.mu.Lock()
		ds.requests = append(ds.requests, ddRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
			Body:   reqBody,
		})
		ds.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ds.Close)
	return ds
}

func (ds *ddServer) only(t *testing.T) ddRequest {
	t.Helper()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(ds.requests))
	}
	return ds.requests[0]
}

func (ds *ddServer) count() int {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return len(ds.requests)
}

// ddAt is a Datadog adapter with both keys set, pointed at baseURL.
func ddAt(baseURL string) *Datadog {
	d := NewDatadog("api-key", "app-key", "")
	d.baseURL = baseURL
	return d
}

func ddAlerts(t *testing.T, env, service string, since time.Time, threshold string, monitors ...ddMonitor) []Alert {
	t.Helper()
	ds := newDDServer(t, http.StatusOK, monitorsBody(t, monitors...))
	got, err := ddAt(ds.URL).GetAlerts(context.Background(), env, service, since, threshold)
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	return got
}

// assertMessages compares the alerts' messages, which are the monitor names.
func assertMessages(t *testing.T, got []Alert, want ...string) {
	t.Helper()
	names := make([]string, 0, len(got))
	for _, a := range got {
		names = append(names, a.Message)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("alerts = %v, want %v", names, want)
	}
}

func assertDDKeys(t *testing.T, r ddRequest) {
	t.Helper()
	if got := r.Header.Get("DD-API-KEY"); got != "api-key" {
		t.Errorf("DD-API-KEY = %q, want %q", got, "api-key")
	}
	if got := r.Header.Get("DD-APPLICATION-KEY"); got != "app-key" {
		t.Errorf("DD-APPLICATION-KEY = %q, want %q", got, "app-key")
	}
}

func TestDatadog_Name(t *testing.T) {
	if got := NewDatadog("api", "app", "").Name(); got != "datadog" {
		t.Errorf("Name() = %q, want datadog", got)
	}
}

func TestDatadog_Active(t *testing.T) {
	cases := []struct {
		name   string
		apiKey string
		appKey string
		want   bool
	}{
		{"both", "api", "app", true},
		{"api only", "api", "", false},
		{"app only", "", "app", false},
		{"neither", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewDatadog(tc.apiKey, tc.appKey, "").Active(); got != tc.want {
				t.Errorf("Active() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDatadog_BaseURL(t *testing.T) {
	if got := NewDatadog("api", "app", "").baseURL; got != "https://api.datadoghq.com" {
		t.Errorf("default baseURL = %q, want https://api.datadoghq.com", got)
	}
	if got := NewDatadog("api", "app", "datadoghq.eu").baseURL; got != "https://api.datadoghq.eu" {
		t.Errorf("custom site baseURL = %q, want https://api.datadoghq.eu", got)
	}
}

func TestDatadog_GetAlerts_Request(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, monitorsBody(t))
	if _, err := ddAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	r := ds.only(t)
	if r.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", r.Method)
	}
	if r.Path != "/api/v1/monitor" {
		t.Errorf("path = %s, want /api/v1/monitor", r.Path)
	}
	assertDDKeys(t, r)
}

func TestDatadog_GetAlerts_StateMapping(t *testing.T) {
	got := ddAlerts(t, "", "", time.Time{}, "",
		ddMon(1, "Alerting", "Alert", ddT0),
		ddMon(2, "Warning", "Warn", ddT0),
		ddMon(3, "Fine", "OK", ddT0),
		ddMon(4, "Silent", "No Data", ddT0),
		ddMon(5, "Ignored", "Ignored", ddT0),
		ddMon(6, "Skipped", "Skipped", ddT0),
		ddMon(7, "Unknown", "Unknown", ddT0),
	)
	assertMessages(t, got, "Alerting", "Warning")
	want := map[string]string{"Alerting": "critical", "Warning": "warning"}
	for _, a := range got {
		if a.State != AlertFiring {
			t.Errorf("%s: State = %q, want %q", a.Message, a.State, AlertFiring)
		}
		if a.Severity != want[a.Message] {
			t.Errorf("%s: Severity = %q, want %q", a.Message, a.Severity, want[a.Message])
		}
	}
}

func TestDatadog_GetAlerts_Mapping(t *testing.T) {
	got := ddAlerts(t, "", "", time.Time{}, "",
		ddMon(4242, "5xx above 5%", "Alert", ddT0, "team:core", "service:checkout", "env:prod"))
	if len(got) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(got))
	}
	a := got[0]
	if a.ID != "datadog:monitor:4242" {
		t.Errorf("ID = %q, want datadog:monitor:4242", a.ID)
	}
	if a.Service != "checkout" {
		t.Errorf("Service = %q, want checkout from the service tag", a.Service)
	}
	if a.Message != "5xx above 5%" {
		t.Errorf("Message = %q, want the monitor name", a.Message)
	}
	if a.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", a.Severity)
	}
	if a.State != AlertFiring {
		t.Errorf("State = %q, want %q", a.State, AlertFiring)
	}
}

func TestDatadog_GetAlerts_NoServiceTagIsEmptyService(t *testing.T) {
	got := ddAlerts(t, "", "", time.Time{}, "", ddMon(1, "Down", "Alert", ddT0, "team:core", "env:prod"))
	if len(got) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(got))
	}
	if got[0].Service != "" {
		t.Errorf("Service = %q, want empty without a service tag", got[0].Service)
	}
}

func TestDatadog_GetAlerts_Time(t *testing.T) {
	modified := ddT0.Add(-6 * time.Hour)
	cases := []struct {
		name    string
		monitor ddMonitor
		want    time.Time
	}{
		{"overall_state_modified wins", ddMonitor{
			ID: 1, Name: "A", OverallState: "Alert",
			OverallStateModified: ddT0.Format(time.RFC3339),
			Modified:             modified.Format(time.RFC3339),
		}, ddT0},
		{"falls back to modified", ddMonitor{
			ID: 2, Name: "B", OverallState: "Alert",
			Modified: modified.Format(time.RFC3339),
		}, modified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ddAlerts(t, "", "", time.Time{}, "", tc.monitor)
			if len(got) != 1 {
				t.Fatalf("expected 1 alert, got %d", len(got))
			}
			if !got[0].Time.Equal(tc.want) {
				t.Errorf("Time = %v, want %v", got[0].Time, tc.want)
			}
		})
	}
}

func TestDatadog_GetAlerts_ServiceFilter(t *testing.T) {
	monitors := []ddMonitor{
		ddMon(1, "A", "Alert", ddT0, "service:checkout"),
		ddMon(2, "B", "Alert", ddT0, "service:payments"),
		ddMon(3, "C", "Alert", ddT0),
	}
	assertMessages(t, ddAlerts(t, "", "checkout", time.Time{}, "", monitors...), "A")
	assertMessages(t, ddAlerts(t, "", "", time.Time{}, "", monitors...), "A", "B", "C")
}

func TestDatadog_GetAlerts_SinceFilter(t *testing.T) {
	monitors := []ddMonitor{
		ddMon(1, "Old", "Alert", ddT0.Add(-2*time.Hour)),
		ddMon(2, "New", "Alert", ddT0.Add(time.Hour)),
	}
	assertMessages(t, ddAlerts(t, "", "", ddT0, "", monitors...), "New")
	assertMessages(t, ddAlerts(t, "", "", time.Time{}, "", monitors...), "New", "Old")
}

func TestDatadog_GetAlerts_ThresholdFilter(t *testing.T) {
	monitors := []ddMonitor{
		ddMon(1, "Critical", "Alert", ddT0),
		ddMon(2, "Warning", "Warn", ddT0),
	}
	cases := []struct {
		threshold string
		want      []string
	}{
		{"", []string{"Critical", "Warning"}},
		{"info", []string{"Critical", "Warning"}},
		{"warning", []string{"Critical", "Warning"}},
		{"error", []string{"Critical"}},
		{"critical", []string{"Critical"}},
		{"CRITICAL", []string{"Critical"}},
	}
	for _, tc := range cases {
		t.Run("threshold="+tc.threshold, func(t *testing.T) {
			assertMessages(t, ddAlerts(t, "", "", time.Time{}, tc.threshold, monitors...), tc.want...)
		})
	}
}

func TestDatadog_GetAlerts_UnknownThresholdIsError(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, monitorsBody(t, ddMon(1, "A", "Alert", ddT0)))
	got, err := ddAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, "loud")
	if err == nil {
		t.Fatalf("expected an error for an unknown threshold, got %d alerts", len(got))
	}
	if !strings.Contains(err.Error(), "loud") {
		t.Errorf("error %q should name the bad threshold", err)
	}
}

func TestDatadog_GetAlerts_EnvFilter(t *testing.T) {
	monitors := []ddMonitor{
		ddMon(1, "Prod", "Alert", ddT0, "env:prod", "service:checkout"),
		ddMon(2, "Staging", "Alert", ddT0, "env:staging", "service:checkout"),
		ddMon(3, "Untagged", "Alert", ddT0, "service:checkout"),
	}
	assertMessages(t, ddAlerts(t, "prod", "", time.Time{}, "", monitors...), "Prod")
	assertMessages(t, ddAlerts(t, "", "", time.Time{}, "", monitors...), "Prod", "Staging", "Untagged")
}

const ddSeriesBody = `{"status":"ok","series":[{"metric":"system.cpu.user","tag_set":["host:web-1","env:prod"],"pointlist":[[1700000000000,1.5],[1700000060000,null],[1700000120500,2]]}]}`

func TestDatadog_FetchMetrics_Request(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, ddSeriesBody)
	since := time.Unix(1700000000, 0)
	_, err := ddAt(ds.URL).FetchMetrics(context.Background(), MetricQuery{
		Query: "avg:system.cpu.user{env:prod}",
		Since: since,
		Step:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("FetchMetrics: %v", err)
	}
	r := ds.only(t)
	if r.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", r.Method)
	}
	if r.Path != "/api/v1/query" {
		t.Errorf("path = %s, want /api/v1/query", r.Path)
	}
	if got := r.Query.Get("query"); got != "avg:system.cpu.user{env:prod}" {
		t.Errorf("query = %q", got)
	}
	if got := r.Query.Get("from"); got != strconv.FormatInt(since.Unix(), 10) {
		t.Errorf("from = %q, want %d (unix seconds)", got, since.Unix())
	}
	to, err := strconv.ParseInt(r.Query.Get("to"), 10, 64)
	if err != nil {
		t.Fatalf("to %q does not parse as unix seconds: %v", r.Query.Get("to"), err)
	}
	if now := time.Now().Unix(); to < now-60 || to > now+60 {
		t.Errorf("to = %d, want about now (%d) in unix seconds", to, now)
	}
	for k := range r.Query {
		if k != "query" && k != "from" && k != "to" {
			t.Errorf("unexpected query parameter %q: Step is ignored", k)
		}
	}
	assertDDKeys(t, r)
}

func TestDatadog_FetchMetrics_Mapping(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, ddSeriesBody)
	got, err := ddAt(ds.URL).FetchMetrics(context.Background(), MetricQuery{Query: "avg:system.cpu.user{*}", Since: time.Unix(1700000000, 0)})
	if err != nil {
		t.Fatalf("FetchMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 series, got %d", len(got))
	}
	s := got[0]
	if s.Name != "system.cpu.user" {
		t.Errorf("Name = %q, want system.cpu.user", s.Name)
	}
	if len(s.Labels) != 2 || s.Labels["host"] != "web-1" || s.Labels["env"] != "prod" {
		t.Errorf("Labels = %v, want host=web-1 and env=prod", s.Labels)
	}
	want := []MetricPoint{
		{Time: time.UnixMilli(1700000000000), Value: 1.5},
		{Time: time.UnixMilli(1700000120500), Value: 2},
	}
	if len(s.Points) != len(want) {
		t.Fatalf("expected %d points (the null one skipped), got %d", len(want), len(s.Points))
	}
	for i, p := range s.Points {
		if !p.Time.Equal(want[i].Time) || p.Value != want[i].Value {
			t.Errorf("point %d = {%v %v}, want {%v %v}", i, p.Time, p.Value, want[i].Time, want[i].Value)
		}
	}
}

const ddLogsBody = `{"data":[{"id":"AQAA","type":"log","attributes":{"timestamp":"2026-10-01T12:00:00.250Z","service":"checkout","message":"payment declined"}},{"id":"AQAB","type":"log","attributes":{"timestamp":"2026-10-01T11:59:00Z","service":"payments","message":"timeout"}}]}`

// ddLogsRequest is the body of a /api/v2/logs/events/search request.
type ddLogsRequest struct {
	Filter struct {
		Query string `json:"query"`
		From  string `json:"from"`
	} `json:"filter"`
	Page struct {
		Limit int `json:"limit"`
	} `json:"page"`
	Sort string `json:"sort"`
}

func ddLogsSent(t *testing.T, q LogQuery) (ddRequest, ddLogsRequest) {
	t.Helper()
	ds := newDDServer(t, http.StatusOK, ddLogsBody)
	if _, err := ddAt(ds.URL).FetchLogs(context.Background(), q); err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	r := ds.only(t)
	var sent ddLogsRequest
	if err := json.Unmarshal(r.Body, &sent); err != nil {
		t.Fatalf("request body %q is not JSON: %v", r.Body, err)
	}
	return r, sent
}

func TestDatadog_FetchLogs_Request(t *testing.T) {
	r, sent := ddLogsSent(t, LogQuery{Service: "checkout", Query: "status:error", Since: ddT0, Limit: 50})
	if r.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.Path != "/api/v2/logs/events/search" {
		t.Errorf("path = %s, want /api/v2/logs/events/search", r.Path)
	}
	if sent.Filter.Query != "status:error" {
		t.Errorf("filter.query = %q, want q.Query", sent.Filter.Query)
	}
	from, err := time.Parse(time.RFC3339, sent.Filter.From)
	if err != nil {
		t.Fatalf("filter.from %q is not RFC3339: %v", sent.Filter.From, err)
	}
	if !from.Equal(ddT0) {
		t.Errorf("filter.from = %v, want %v", from, ddT0)
	}
	if sent.Page.Limit != 50 {
		t.Errorf("page.limit = %d, want 50", sent.Page.Limit)
	}
	if sent.Sort != "-timestamp" {
		t.Errorf("sort = %q, want -timestamp", sent.Sort)
	}
	assertDDKeys(t, r)
}

func TestDatadog_FetchLogs_QueryFallsBackToService(t *testing.T) {
	_, sent := ddLogsSent(t, LogQuery{Service: "checkout", Since: ddT0})
	if sent.Filter.Query != "service:checkout" {
		t.Errorf("filter.query = %q, want service:checkout", sent.Filter.Query)
	}
}

func TestDatadog_FetchLogs_NoQueryNoServiceMakesNoRequest(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, ddLogsBody)
	got, err := ddAt(ds.URL).FetchLogs(context.Background(), LogQuery{Since: ddT0})
	if err == nil {
		t.Fatalf("expected an error without a query or a service, got %d lines", len(got))
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("error %q should say the query is missing", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no log lines alongside the error, got %d", len(got))
	}
	if n := ds.count(); n != 0 {
		t.Errorf("empty query made %d requests, want 0", n)
	}
}

func TestDatadog_FetchLogs_Limit(t *testing.T) {
	cases := []struct {
		limit int
		want  int
	}{
		{0, 200},
		{50, 50},
		{1000, 1000},
		{5000, 1000},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("limit=%d", tc.limit), func(t *testing.T) {
			_, sent := ddLogsSent(t, LogQuery{Query: "status:error", Since: ddT0, Limit: tc.limit})
			if sent.Page.Limit != tc.want {
				t.Errorf("page.limit = %d, want %d", sent.Page.Limit, tc.want)
			}
		})
	}
}

func TestDatadog_FetchLogs_Mapping(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, ddLogsBody)
	got, err := ddAt(ds.URL).FetchLogs(context.Background(), LogQuery{Query: "status:error", Since: ddT0})
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	want := []LogLine{
		{Time: time.Date(2026, 10, 1, 12, 0, 0, 250_000_000, time.UTC), Service: "checkout", Message: "payment declined"},
		{Time: time.Date(2026, 10, 1, 11, 59, 0, 0, time.UTC), Service: "payments", Message: "timeout"},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d log lines, got %d", len(want), len(got))
	}
	for i, l := range got {
		if !l.Time.Equal(want[i].Time) || l.Service != want[i].Service || l.Message != want[i].Message {
			t.Errorf("line %d = {%v %q %q}, want {%v %q %q}", i, l.Time, l.Service, l.Message, want[i].Time, want[i].Service, want[i].Message)
		}
	}
}

// ddCalls runs the three HTTP-backed calls against a server, so each failure
// mode is pinned for alerts, metrics and logs.
var ddCalls = []struct {
	name string
	call func(d *Datadog) (int, error)
}{
	{"GetAlerts", func(d *Datadog) (int, error) {
		got, err := d.GetAlerts(context.Background(), "", "", time.Time{}, "")
		return len(got), err
	}},
	{"FetchMetrics", func(d *Datadog) (int, error) {
		got, err := d.FetchMetrics(context.Background(), MetricQuery{Query: "avg:system.cpu.user{*}", Since: time.Unix(1700000000, 0)})
		return len(got), err
	}},
	{"FetchLogs", func(d *Datadog) (int, error) {
		got, err := d.FetchLogs(context.Background(), LogQuery{Query: "status:error", Since: ddT0})
		return len(got), err
	}},
}

func TestDatadog_NonSuccessStatusIsError(t *testing.T) {
	for _, tc := range ddCalls {
		t.Run(tc.name, func(t *testing.T) {
			ds := newDDServer(t, http.StatusForbidden, `{"errors":["Forbidden"]}`)
			n, err := tc.call(ddAt(ds.URL))
			if err == nil {
				t.Fatalf("expected an error for a 403, got %d results", n)
			}
			if ds.count() != 1 {
				t.Fatalf("expected the call to reach the server once, got %d requests", ds.count())
			}
			if !strings.Contains(err.Error(), "datadog") {
				t.Errorf("error %q should name datadog", err)
			}
			if !strings.Contains(err.Error(), "403") {
				t.Errorf("error %q should include the status code", err)
			}
			if n != 0 {
				t.Errorf("expected no results alongside the error, got %d", n)
			}
		})
	}
}

func TestDatadog_OversizedBodyIsError(t *testing.T) {
	// Valid JSON on purpose: an adapter that reads without a cap would decode
	// it and succeed.
	const pad = 9 << 20
	bodies := map[string][2]string{
		"GetAlerts":    {`[{"id":1,"name":"`, `","overall_state":"OK","tags":[]}]`},
		"FetchMetrics": {`{"status":"ok","series":[],"pad":"`, `"}`},
		"FetchLogs":    {`{"data":[],"pad":"`, `"}`},
	}
	chunk := []byte(strings.Repeat("a", 64<<10))
	for _, tc := range ddCalls {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			parts := bodies[tc.name]
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
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

			n, err := tc.call(ddAt(srv.URL))
			if err == nil {
				t.Fatalf("expected an error for a body over 8 MiB, got %d results", n)
			}
			if hits.Load() != 1 {
				t.Fatalf("expected the call to reach the server once, got %d requests", hits.Load())
			}
			if !strings.Contains(err.Error(), "datadog") {
				t.Errorf("error %q should name datadog", err)
			}
		})
	}
}
