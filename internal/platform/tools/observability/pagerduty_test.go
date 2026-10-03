package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var _ ObservabilityAdapter = (*PagerDuty)(nil)

var pdT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// pdIncident is one entry of PagerDuty's /incidents response.
type pdIncident struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Urgency   string    `json:"urgency"`
	CreatedAt string    `json:"created_at"`
	Service   pdService `json:"service"`
}

type pdService struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

// pdInc is an incident created at pdT0 on the service named service.
func pdInc(id, title, status, urgency, service string) pdIncident {
	return pdIncident{
		ID:        id,
		Title:     title,
		Status:    status,
		Urgency:   urgency,
		CreatedAt: pdT0.Format(time.RFC3339),
		Service:   pdService{ID: "S" + id, Summary: service},
	}
}

func incidentsBody(t *testing.T, more bool, incidents ...pdIncident) string {
	t.Helper()
	if incidents == nil {
		incidents = []pdIncident{}
	}
	b, err := json.Marshal(struct {
		Incidents []pdIncident `json:"incidents"`
		Limit     int          `json:"limit"`
		Offset    int          `json:"offset"`
		More      bool         `json:"more"`
	}{Incidents: incidents, Limit: 100, More: more})
	if err != nil {
		t.Fatalf("marshal incidents: %v", err)
	}
	return string(b)
}

// pdAt is a PagerDuty adapter with a token set, pointed at baseURL.
func pdAt(baseURL string) *PagerDuty {
	p := NewPagerDuty("pd-token")
	p.baseURL = baseURL
	return p
}

func pdAlerts(t *testing.T, env, service string, since time.Time, threshold string, incidents ...pdIncident) []Alert {
	t.Helper()
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, false, incidents...))
	got, err := pdAt(ds.URL).GetAlerts(context.Background(), env, service, since, threshold)
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	return got
}

func TestPagerDuty_Name(t *testing.T) {
	if got := NewPagerDuty("tok").Name(); got != "pagerduty" {
		t.Errorf("Name() = %q, want pagerduty", got)
	}
}

func TestPagerDuty_Active(t *testing.T) {
	if !NewPagerDuty("tok").Active() {
		t.Errorf("Active() = false with a token, want true")
	}
	if NewPagerDuty("").Active() {
		t.Errorf("Active() = true without a token, want false")
	}
}

func TestPagerDuty_BaseURL(t *testing.T) {
	if got := NewPagerDuty("tok").baseURL; got != "https://api.pagerduty.com" {
		t.Errorf("default baseURL = %q, want https://api.pagerduty.com", got)
	}
}

func TestPagerDuty_GetAlerts_Request(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, false))
	if _, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	r := ds.only(t)
	if r.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", r.Method)
	}
	if r.Path != "/incidents" {
		t.Errorf("path = %s, want /incidents", r.Path)
	}
	if got := r.Header.Get("Authorization"); got != "Token token=pd-token" {
		t.Errorf("Authorization = %q, want %q", got, "Token token=pd-token")
	}
	statuses := append([]string(nil), r.Query["statuses[]"]...)
	sort.Strings(statuses)
	if strings.Join(statuses, ",") != "acknowledged,triggered" {
		t.Errorf("statuses[] = %v, want exactly triggered and acknowledged", r.Query["statuses[]"])
	}
	if got := r.Query.Get("limit"); got != "100" {
		t.Errorf("limit = %q, want 100", got)
	}
}

func TestPagerDuty_GetAlerts_SinceParameter(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, false))
	if _, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", pdT0, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	raw := ds.only(t).Query.Get("since")
	since, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("since %q is not RFC3339: %v", raw, err)
	}
	if !since.Equal(pdT0) {
		t.Errorf("since = %v, want %v", since, pdT0)
	}
}

func TestPagerDuty_GetAlerts_ZeroSinceSendsNoSince(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, false))
	if _, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, ""); err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	if got, ok := ds.only(t).Query["since"]; ok {
		t.Errorf("since = %v, want no since parameter for the zero time", got)
	}
}

func TestPagerDuty_GetAlerts_StateMapping(t *testing.T) {
	got := pdAlerts(t, "", "", time.Time{}, "",
		pdInc("P1", "Triggered", "triggered", "high", "checkout"),
		pdInc("P2", "Acked", "acknowledged", "high", "checkout"),
	)
	assertMessages(t, got, "Acked", "Triggered")
	want := map[string]string{"Triggered": AlertFiring, "Acked": AlertAcknowledged}
	for _, a := range got {
		if a.State != want[a.Message] {
			t.Errorf("%s: State = %q, want %q", a.Message, a.State, want[a.Message])
		}
	}
}

func TestPagerDuty_GetAlerts_Mapping(t *testing.T) {
	got := pdAlerts(t, "", "", time.Time{}, "", pdInc("PT4KHLK", "Checkout 5xx above 5%", "triggered", "high", "checkout"))
	if len(got) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(got))
	}
	a := got[0]
	if a.ID != "pagerduty:incident:PT4KHLK" {
		t.Errorf("ID = %q, want pagerduty:incident:PT4KHLK", a.ID)
	}
	if a.Service != "checkout" {
		t.Errorf("Service = %q, want checkout from service.summary", a.Service)
	}
	if a.Message != "Checkout 5xx above 5%" {
		t.Errorf("Message = %q, want the incident title", a.Message)
	}
	if !a.Time.Equal(pdT0) {
		t.Errorf("Time = %v, want created_at %v", a.Time, pdT0)
	}
	if a.State != AlertFiring {
		t.Errorf("State = %q, want %q", a.State, AlertFiring)
	}
}

func TestPagerDuty_GetAlerts_SeverityFromUrgency(t *testing.T) {
	got := pdAlerts(t, "", "", time.Time{}, "",
		pdInc("P1", "High", "triggered", "high", "checkout"),
		pdInc("P2", "Low", "triggered", "low", "checkout"),
	)
	assertMessages(t, got, "High", "Low")
	want := map[string]string{"High": "critical", "Low": "warning"}
	for _, a := range got {
		if a.Severity != want[a.Message] {
			t.Errorf("%s: Severity = %q, want %q", a.Message, a.Severity, want[a.Message])
		}
	}
}

func TestPagerDuty_GetAlerts_ServiceFilter(t *testing.T) {
	incidents := []pdIncident{
		pdInc("P1", "A", "triggered", "high", "checkout"),
		pdInc("P2", "B", "triggered", "high", "payments"),
		pdInc("P3", "C", "acknowledged", "high", ""),
	}
	assertMessages(t, pdAlerts(t, "", "checkout", time.Time{}, "", incidents...), "A")
	assertMessages(t, pdAlerts(t, "", "", time.Time{}, "", incidents...), "A", "B", "C")
}

func TestPagerDuty_GetAlerts_ThresholdFilter(t *testing.T) {
	incidents := []pdIncident{
		pdInc("P1", "Critical", "triggered", "high", "checkout"),
		pdInc("P2", "Warning", "triggered", "low", "checkout"),
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
			assertMessages(t, pdAlerts(t, "", "", time.Time{}, tc.threshold, incidents...), tc.want...)
		})
	}
}

func TestPagerDuty_GetAlerts_UnknownThresholdIsError(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, false, pdInc("P1", "A", "triggered", "high", "checkout")))
	got, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, "loud")
	if err == nil {
		t.Fatalf("expected an error for an unknown threshold, got %d alerts", len(got))
	}
	if !strings.Contains(err.Error(), "loud") {
		t.Errorf("error %q should name the bad threshold", err)
	}
}

func TestPagerDuty_GetAlerts_EnvIsIgnored(t *testing.T) {
	incidents := []pdIncident{
		pdInc("P1", "A", "triggered", "high", "checkout"),
		pdInc("P2", "B", "acknowledged", "low", "payments"),
	}
	assertMessages(t, pdAlerts(t, "", "", time.Time{}, "", incidents...), "A", "B")
	assertMessages(t, pdAlerts(t, "prod", "", time.Time{}, "", incidents...), "A", "B")
}

func TestPagerDuty_GetAlerts_MoreIsError(t *testing.T) {
	ds := newDDServer(t, http.StatusOK, incidentsBody(t, true,
		pdInc("P1", "A", "triggered", "high", "checkout"),
		pdInc("P2", "B", "triggered", "high", "checkout"),
	))
	got, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, "")
	if err == nil {
		t.Fatalf("expected an error when more incidents remain, got %d alerts", len(got))
	}
	if ds.count() != 1 {
		t.Fatalf("expected the call to reach the server once, got %d requests", ds.count())
	}
	if !strings.Contains(err.Error(), "100") {
		t.Errorf("error %q should name the limit (100)", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no alerts alongside the error, got %d: a partial set must not look complete", len(got))
	}
}

func TestPagerDuty_GetAlerts_NonSuccessStatusIsError(t *testing.T) {
	ds := newDDServer(t, http.StatusUnauthorized, `{"error":{"message":"Unauthorized"}}`)
	got, err := pdAt(ds.URL).GetAlerts(context.Background(), "", "", time.Time{}, "")
	if err == nil {
		t.Fatalf("expected an error for a 401, got %d alerts", len(got))
	}
	if ds.count() != 1 {
		t.Fatalf("expected the call to reach the server once, got %d requests", ds.count())
	}
	if !strings.Contains(err.Error(), "pagerduty") {
		t.Errorf("error %q should name pagerduty", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q should include the status code", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no alerts alongside the error, got %d", len(got))
	}
}

func TestPagerDuty_GetAlerts_OversizedBodyIsError(t *testing.T) {
	// Valid JSON on purpose: an adapter that reads without a cap would decode
	// it and succeed.
	const pad = 9 << 20
	chunk := []byte(strings.Repeat("a", 64<<10))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"incidents":[],"more":false,"pad":"`)
		for written := 0; written < pad; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		_, _ = fmt.Fprint(w, `"}`)
	}))
	t.Cleanup(srv.Close)

	got, err := pdAt(srv.URL).GetAlerts(context.Background(), "", "", time.Time{}, "")
	if err == nil {
		t.Fatalf("expected an error for a body over 8 MiB, got %d alerts", len(got))
	}
	if hits.Load() != 1 {
		t.Fatalf("expected the call to reach the server once, got %d requests", hits.Load())
	}
	if !strings.Contains(err.Error(), "pagerduty") {
		t.Errorf("error %q should name pagerduty", err)
	}
}

func TestPagerDuty_LogsAndMetricsAreUnsupported(t *testing.T) {
	calls := map[string]func(p *PagerDuty) (int, error){
		"FetchLogs": func(p *PagerDuty) (int, error) {
			got, err := p.FetchLogs(context.Background(), LogQuery{Service: "checkout", Since: pdT0})
			return len(got), err
		},
		"FetchMetrics": func(p *PagerDuty) (int, error) {
			got, err := p.FetchMetrics(context.Background(), MetricQuery{Query: "up", Since: pdT0})
			return len(got), err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			ds := newDDServer(t, http.StatusOK, incidentsBody(t, false))
			n, err := call(pdAt(ds.URL))
			if err == nil {
				t.Fatalf("expected ErrUnsupported, got %d results and a nil error", n)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("errors.Is(err, ErrUnsupported) = false for %v", err)
			}
			if !strings.Contains(err.Error(), "pagerduty") {
				t.Errorf("error %q should name pagerduty", err)
			}
			if c := ds.count(); c != 0 {
				t.Errorf("expected no request, got %d", c)
			}
		})
	}
}
