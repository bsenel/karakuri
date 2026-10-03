package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var _ ObservabilityAdapter = (*Loki)(nil)

// lokiStreams is the query_range response the mapping tests decode: two
// streams whose lines interleave in time.
const lokiStreams = `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service":"api"},"values":[["1700000002000000000","second"],["1700000001000000000","first"]]},{"stream":{"service":"web"},"values":[["1700000003000000000","third"]]}]}}`

const lokiEmpty = `{"status":"success","data":{"resultType":"streams","result":[]}}`

var lokiSince = time.Unix(1700000000, 123456789)

type lokiServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newLokiServer(t *testing.T, status int, body string) *lokiServer {
	t.Helper()
	ls := &lokiServer{}
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		ls.requests = append(ls.requests, r)
		ls.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(ls.Close)
	return ls
}

func (ls *lokiServer) count() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.requests)
}

// only returns the single request the server received.
func (ls *lokiServer) only(t *testing.T) *http.Request {
	t.Helper()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if len(ls.requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(ls.requests))
	}
	return ls.requests[0]
}

func TestLoki_Name(t *testing.T) {
	if got := NewLoki("http://loki:3100", "", "").Name(); got != "loki" {
		t.Errorf("Name() = %q, want loki", got)
	}
}

func TestLoki_Active(t *testing.T) {
	if NewLoki("", "tok", "acme").Active() {
		t.Errorf("Loki without a url should be inactive")
	}
	if !NewLoki("http://loki:3100", "", "").Active() {
		t.Errorf("Loki with a url should be active")
	}
}

func TestLoki_FetchLogs_Request(t *testing.T) {
	srv := newLokiServer(t, http.StatusOK, lokiEmpty)
	l := NewLoki(srv.URL+"/", "tok", "acme")

	if _, err := l.FetchLogs(context.Background(), LogQuery{Query: `{app="api"} |= "error"`, Since: lokiSince, Limit: 50}); err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	req := srv.only(t)
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if req.URL.Path != "/loki/api/v1/query_range" {
		t.Errorf("path = %q, want /loki/api/v1/query_range", req.URL.Path)
	}
	params := req.URL.Query()
	if got := params.Get("query"); got != `{app="api"} |= "error"` {
		t.Errorf("query = %q, want the LogQL verbatim", got)
	}
	if got, want := params.Get("start"), strconv.FormatInt(lokiSince.UnixNano(), 10); got != want {
		t.Errorf("start = %q, want %q (unix nanoseconds)", got, want)
	}
	if got := params.Get("limit"); got != "50" {
		t.Errorf("limit = %q, want 50", got)
	}
	if got := params.Get("direction"); got != "backward" {
		t.Errorf("direction = %q, want backward", got)
	}
}

func TestLoki_FetchLogs_QuerySelection(t *testing.T) {
	cases := []struct {
		name string
		q    LogQuery
		want string
	}{
		{"query verbatim", LogQuery{Query: `{app="api"} |~ "time.out"`}, `{app="api"} |~ "time.out"`},
		{"query wins over service", LogQuery{Query: `{app="api"}`, Service: "web"}, `{app="api"}`},
		{"service selector", LogQuery{Service: "web"}, `{service="web"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLokiServer(t, http.StatusOK, lokiEmpty)
			tc.q.Since = lokiSince
			if _, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), tc.q); err != nil {
				t.Fatalf("FetchLogs: %v", err)
			}
			if got := srv.only(t).URL.Query().Get("query"); got != tc.want {
				t.Errorf("query = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoki_FetchLogs_NoQueryNoServiceMakesNoRequest(t *testing.T) {
	srv := newLokiServer(t, http.StatusOK, lokiStreams)
	lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Since: lokiSince})
	if err == nil {
		t.Fatalf("expected an error without a query or a service, got %d lines", len(lines))
	}
	if errors.Is(err, ErrUnsupported) {
		t.Errorf("a missing query is not an unsupported signal: %v", err)
	}
	if n := srv.count(); n != 0 {
		t.Errorf("expected no request, got %d", n)
	}
}

func TestLoki_FetchLogs_Limit(t *testing.T) {
	cases := []struct {
		limit int
		want  string
	}{
		{0, "200"},
		{-5, "200"},
		{1, "1"},
		{37, "37"},
		{1000, "1000"},
		{1001, "1000"},
		{50000, "1000"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.limit), func(t *testing.T) {
			srv := newLokiServer(t, http.StatusOK, lokiEmpty)
			q := LogQuery{Service: "api", Since: lokiSince, Limit: tc.limit}
			if _, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), q); err != nil {
				t.Fatalf("FetchLogs: %v", err)
			}
			if got := srv.only(t).URL.Query().Get("limit"); got != tc.want {
				t.Errorf("limit sent = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoki_FetchLogs_Headers(t *testing.T) {
	cases := []struct {
		name, token, tenant string
	}{
		{"token and tenant", "tok", "acme"},
		{"token only", "tok", ""},
		{"tenant only", "", "acme"},
		{"neither", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLokiServer(t, http.StatusOK, lokiEmpty)
			l := NewLoki(srv.URL, tc.token, tc.tenant)
			if _, err := l.FetchLogs(context.Background(), LogQuery{Service: "api", Since: lokiSince}); err != nil {
				t.Fatalf("FetchLogs: %v", err)
			}
			h := srv.only(t).Header

			wantAuth := ""
			if tc.token != "" {
				wantAuth = "Bearer " + tc.token
			}
			if got := h.Get("Authorization"); got != wantAuth {
				t.Errorf("Authorization = %q, want %q", got, wantAuth)
			}
			if _, present := h["Authorization"]; present != (tc.token != "") {
				t.Errorf("Authorization present = %t, want %t", present, tc.token != "")
			}
			if got := h.Get("X-Scope-OrgID"); got != tc.tenant {
				t.Errorf("X-Scope-OrgID = %q, want %q", got, tc.tenant)
			}
			if _, present := h[http.CanonicalHeaderKey("X-Scope-OrgID")]; present != (tc.tenant != "") {
				t.Errorf("X-Scope-OrgID present = %t, want %t", present, tc.tenant != "")
			}
		})
	}
}

func TestLoki_FetchLogs_Mapping(t *testing.T) {
	srv := newLokiServer(t, http.StatusOK, lokiStreams)
	lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Query: `{env="prod"}`, Since: lokiSince})
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	// Newest first across streams, not stream by stream.
	want := []LogLine{
		{Time: time.Unix(0, 1700000003000000000), Service: "web", Message: "third"},
		{Time: time.Unix(0, 1700000002000000000), Service: "api", Message: "second"},
		{Time: time.Unix(0, 1700000001000000000), Service: "api", Message: "first"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, w := range want {
		got := lines[i]
		if !got.Time.Equal(w.Time) {
			t.Errorf("line %d time = %s, want %s", i, got.Time, w.Time)
		}
		if got.Service != w.Service {
			t.Errorf("line %d service = %q, want %q", i, got.Service, w.Service)
		}
		if got.Message != w.Message {
			t.Errorf("line %d message = %q, want %q", i, got.Message, w.Message)
		}
	}
}

func TestLoki_FetchLogs_ServiceFallsBackToQueryService(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"streams","result":[` +
		`{"stream":{"pod":"api-1"},"values":[["1700000001000000000","unlabelled"]]},` +
		`{"stream":{"service":"web"},"values":[["1700000002000000000","labelled"]]}]}}`
	srv := newLokiServer(t, http.StatusOK, body)
	lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Service: "api", Since: lokiSince})
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].Message != "labelled" || lines[0].Service != "web" {
		t.Errorf("line 0 = %+v, want the stream's own service label web", lines[0])
	}
	if lines[1].Message != "unlabelled" || lines[1].Service != "api" {
		t.Errorf("line 1 = %+v, want the query's service api as fallback", lines[1])
	}
}

func TestLoki_GetAlertsAndFetchMetrics_Unsupported(t *testing.T) {
	calls := map[string]func(l *Loki) (int, error){
		"GetAlerts": func(l *Loki) (int, error) {
			alerts, err := l.GetAlerts(context.Background(), "prod", "api", lokiSince, "warning")
			return len(alerts), err
		},
		"FetchMetrics": func(l *Loki) (int, error) {
			series, err := l.FetchMetrics(context.Background(), MetricQuery{Query: "up", Since: lokiSince})
			return len(series), err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			srv := newLokiServer(t, http.StatusOK, lokiStreams)
			n, err := call(NewLoki(srv.URL, "tok", "acme"))
			if err == nil {
				t.Fatalf("expected ErrUnsupported, got %d results and a nil error", n)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("errors.Is(err, ErrUnsupported) = false for %v", err)
			}
			if !strings.Contains(err.Error(), "loki") {
				t.Errorf("error %q should name loki", err)
			}
			if c := srv.count(); c != 0 {
				t.Errorf("expected no request, got %d", c)
			}
		})
	}
}

func TestLoki_FetchLogs_NonSuccessStatusIsError(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			// A well-formed body on purpose: the status alone must fail the call.
			srv := newLokiServer(t, status, lokiStreams)
			lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Service: "api", Since: lokiSince})
			if err == nil {
				t.Fatalf("expected an error for status %d, got %d lines", status, len(lines))
			}
			if !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("error %q should include the status code %d", err, status)
			}
			if srv.count() != 1 {
				t.Errorf("expected the call to reach the server once, got %d requests", srv.count())
			}
		})
	}
}

func TestLoki_FetchLogs_ErrorStatusBodyIsError(t *testing.T) {
	body := `{"status":"error","errorType":"bad_data","error":"parse error at line 1","data":{"resultType":"streams","result":[]}}`
	srv := newLokiServer(t, http.StatusOK, body)
	lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Service: "api", Since: lokiSince})
	if err == nil {
		t.Fatalf(`expected an error for "status":"error", got %d lines`, len(lines))
	}
	if srv.count() != 1 {
		t.Errorf("expected the call to reach the server once, got %d requests", srv.count())
	}
}

func TestLoki_FetchLogs_OversizedBodyIsError(t *testing.T) {
	// Valid JSON on purpose: an adapter that reads without a cap would decode
	// it and succeed.
	const pad = 9 << 20
	chunk := []byte(strings.Repeat("a", 64<<10))
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"streams","result":[],"pad":"`)
		for written := 0; written < pad; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		_, _ = fmt.Fprint(w, `"}}`)
	}))
	t.Cleanup(srv.Close)

	lines, err := NewLoki(srv.URL, "", "").FetchLogs(context.Background(), LogQuery{Service: "api", Since: lokiSince})
	if err == nil {
		t.Fatalf("expected an error for a body over 8 MiB, got %d lines", len(lines))
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("expected the call to reach the server once, got %d requests", hits)
	}
}
