package software

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

// Phase 41 step 12: p95_latency_ms and error_rate through Phase 32's
// observability adapters (docs/roadmap.md, Phase 41 section 7 item 7 and step
// 12). Recorded Prometheus and Datadog responses give the value; an empty
// series is inconclusive: no_data, never 0 and never met.

// runtimeVerifier is one of the two step 12 verifiers and what it measures.
type runtimeVerifier struct {
	capID  string
	metric string
	unit   string
}

var runtimeVerifiers = []runtimeVerifier{
	{CapP95Latency, "p95_latency_ms", "ms"},
	{CapErrorRate, "error_rate", "ratio"},
}

// promMatrix is a recorded Prometheus /api/v1/query_range response with one
// series whose points are values, oldest first.
func promMatrix(values ...string) string {
	points := ""
	for i, v := range values {
		if i > 0 {
			points += ","
		}
		points += fmt.Sprintf(`[%d,"%s"]`, 1700000000+60*i, v)
	}
	return `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"api","service":"checkout"},"values":[` + points + `]}]}}`
}

const (
	promEmptyMatrix = `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	promNoPoints    = `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"api"},"values":[]}]}}`
	promTwoSeries   = `{"status":"success","data":{"resultType":"matrix","result":[` +
		`{"metric":{"job":"api","instance":"a"},"values":[[1700000000,"250"]]},` +
		`{"metric":{"job":"api","instance":"b"},"values":[[1700000000,"410"]]}]}}`
)

// ddSeries is a recorded Datadog /api/v1/query response with one series whose
// points are values, oldest first.
func ddSeries(values ...string) string {
	points := ""
	for i, v := range values {
		if i > 0 {
			points += ","
		}
		points += fmt.Sprintf(`[%d,%s]`, int64(1700000000000)+int64(60000*i), v)
	}
	return `{"status":"ok","series":[{"metric":"trace.http.request","tag_set":["service:checkout","env:prod"],"pointlist":[` + points + `]}]}`
}

const ddEmpty = `{"status":"ok","series":[]}`

// recordedBackend serves body at path and counts the requests it was asked.
type recordedBackend struct {
	*httptest.Server
	requests int
	lastURL  *url.URL
}

func serveRecorded(t *testing.T, path string, status int, body string) *recordedBackend {
	t.Helper()
	rb := &recordedBackend{}
	rb.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rb.requests++
		rb.lastURL = r.URL
		if r.URL.Path != path {
			t.Errorf("request to %s, want %s", r.URL.Path, path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(rb.Close)
	return rb
}

// prometheusEnv is the observability environment bound to a real Prometheus
// adapter that is served a recorded query_range response.
func prometheusEnv(t *testing.T, status int, body string) (*observabilityEnv, *recordedBackend) {
	t.Helper()
	rb := serveRecorded(t, "/api/v1/query_range", status, body)
	return newObservabilityEnv(envObservability, observability.NewPrometheus(rb.URL, "")), rb
}

// redirectTransport sends every request to target, so the Datadog adapter,
// whose base URL is always https://api.<site>, reaches a recorded server.
type redirectTransport struct {
	target *url.URL
	next   http.RoundTripper
}

func (rt redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host, r.Host = rt.target.Scheme, rt.target.Host, rt.target.Host
	return rt.next.RoundTrip(r)
}

// datadogEnv is the observability environment bound to a real Datadog adapter
// that is served a recorded /api/v1/query response. The adapter's client uses
// http.DefaultTransport, which is redirected for this test only; the tests that
// use it do not run in parallel.
func datadogEnv(t *testing.T, status int, body string) (*observabilityEnv, *recordedBackend) {
	t.Helper()
	rb := serveRecorded(t, "/api/v1/query", status, body)
	target, err := url.Parse(rb.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	http.DefaultTransport = redirectTransport{target: target, next: original}
	t.Cleanup(func() { http.DefaultTransport = original })
	return newObservabilityEnv(envObservability, observability.NewDatadog("api-key", "app-key", "datadoghq.test")), rb
}

func measure(t *testing.T, env *observabilityEnv, capID string, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: capability.CapabilityID(capID), Params: params})
	if err != nil {
		t.Fatalf("Act(%s) returned an error; a verifier reports what it could not measure in its result: %v", capID, err)
	}
	return res
}

// assertMeasured requires exactly one first-party measurement of the metric,
// in its unit, with the exact value.
func assertMeasured(t *testing.T, res environment.ActionResult, v runtimeVerifier, want float64) {
	t.Helper()
	if !res.Success {
		t.Fatalf("%s did not succeed: %s", v.capID, res.Error)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("%s emitted %d measurements, want exactly 1: %+v", v.capID, len(res.Measurements), res.Measurements)
	}
	m := res.Measurements[0]
	if m.Name != v.metric || m.Unit != v.unit {
		t.Errorf("measurement = %s in %q, want %s in %q", m.Name, m.Unit, v.metric, v.unit)
	}
	if m.Value != want {
		t.Errorf("%s = %v, want %v, the latest point of the series", v.metric, m.Value, want)
	}
	if m.Source == "" {
		t.Error("the measurement names no source")
	}
	// The value comes from this deployment's own bound backend, like
	// fetchMetrics' numbers: first-party, so it can settle a required criterion.
	if m.Trust != environment.TrustOperator {
		t.Errorf("measurement trust = %q, want %q", m.Trust, environment.TrustOperator)
	}
	if res.Trust.IsThirdParty() {
		t.Errorf("result trust = %q: a number from the bound backend is nobody else's writing", res.Trust)
	}
}

// assertInconclusive requires the verifier's own inconclusive verdict with
// reason, and no measurement: nothing it could not measure reads as a value.
func assertInconclusive(t *testing.T, res environment.ActionResult, v runtimeVerifier, reason objective.InconclusiveReason) {
	t.Helper()
	if res.Success {
		t.Errorf("%s succeeded on a result it could not measure", v.capID)
	}
	if len(res.Measurements) != 0 {
		t.Errorf("%s emitted %+v; an unmeasured value is never reported, as 0 or otherwise", v.capID, res.Measurements)
	}
	if got, _ := res.StateDelta["state"].(string); got != string(objective.CriterionInconclusive) {
		t.Errorf("state = %q, want %q (StateDelta %v)", got, objective.CriterionInconclusive, res.StateDelta)
	}
	if got, _ := res.StateDelta["reason"].(string); got != string(reason) {
		t.Errorf("reason = %q, want %q (StateDelta %v)", got, reason, res.StateDelta)
	}
	if res.Error == "" {
		t.Error("an inconclusive result says nothing about why")
	}
}

// (a) A recorded Prometheus query_range response gives the exact value: the
// latest point, not the first, a mean or a maximum.
func TestRuntimeVerifierReadsRecordedPrometheus(t *testing.T) {
	cases := []struct {
		v      runtimeVerifier
		values []string
		want   float64
	}{
		{runtimeVerifiers[0], []string{"310", "298.25", "287.5"}, 287.5},
		{runtimeVerifiers[1], []string{"0.05", "0.0125"}, 0.0125},
	}
	for _, tc := range cases {
		t.Run(tc.v.metric, func(t *testing.T) {
			env, rb := prometheusEnv(t, http.StatusOK, promMatrix(tc.values...))
			query := `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_ms_bucket[5m])))`

			res := measure(t, env, tc.v.capID, map[string]any{"query": query})

			assertMeasured(t, res, tc.v, tc.want)
			if rb.requests != 1 {
				t.Fatalf("Prometheus was asked %d times, want once", rb.requests)
			}
			if got := rb.lastURL.Query().Get("query"); got != query {
				t.Errorf("query sent = %q, want the caller's %q verbatim", got, query)
			}
		})
	}
}

// (b) A recorded Datadog response gives the exact value, a gap (null) before
// the latest point notwithstanding.
func TestRuntimeVerifierReadsRecordedDatadog(t *testing.T) {
	cases := []struct {
		v      runtimeVerifier
		values []string
		want   float64
	}{
		{runtimeVerifiers[0], []string{"402", "null", "251.75"}, 251.75},
		{runtimeVerifiers[1], []string{"0.031", "0.004"}, 0.004},
	}
	for _, tc := range cases {
		t.Run(tc.v.metric, func(t *testing.T) {
			env, rb := datadogEnv(t, http.StatusOK, ddSeries(tc.values...))
			query := "p95:trace.http.request{service:checkout}"

			res := measure(t, env, tc.v.capID, map[string]any{"query": query})

			assertMeasured(t, res, tc.v, tc.want)
			if rb.requests != 1 {
				t.Fatalf("Datadog was asked %d times, want once", rb.requests)
			}
			if got := rb.lastURL.Query().Get("query"); got != query {
				t.Errorf("query sent = %q, want the caller's %q verbatim", got, query)
			}
		})
	}
}

// (c) No data is inconclusive: no_data, never 0 and never met.
func TestRuntimeVerifierWithNoDataIsInconclusive(t *testing.T) {
	cases := map[string]func(t *testing.T) *observabilityEnv{
		"prometheus empty series": func(t *testing.T) *observabilityEnv {
			env, _ := prometheusEnv(t, http.StatusOK, promEmptyMatrix)
			return env
		},
		"prometheus series with no points": func(t *testing.T) *observabilityEnv {
			env, _ := prometheusEnv(t, http.StatusOK, promNoPoints)
			return env
		},
		"prometheus NaN at the latest point": func(t *testing.T) *observabilityEnv {
			env, _ := prometheusEnv(t, http.StatusOK, promMatrix("120", "NaN"))
			return env
		},
		"datadog empty series": func(t *testing.T) *observabilityEnv {
			env, _ := datadogEnv(t, http.StatusOK, ddEmpty)
			return env
		},
		"datadog series of gaps only": func(t *testing.T) *observabilityEnv {
			env, _ := datadogEnv(t, http.StatusOK, ddSeries("null", "null"))
			return env
		},
	}
	for name, build := range cases {
		for _, v := range runtimeVerifiers {
			t.Run(name+"/"+v.metric, func(t *testing.T) {
				res := measure(t, build(t), v.capID, map[string]any{"query": "q"})
				assertInconclusive(t, res, v, objective.ReasonNoData)
			})
		}
	}
}

// (d) A backend with no metrics says so, and that is inconclusive:
// unsupported, never an empty result read as zero (AGENTS.md rule 10).
func TestRuntimeVerifierOnABackendWithoutMetricsIsUnsupported(t *testing.T) {
	for _, v := range runtimeVerifiers {
		t.Run(v.metric, func(t *testing.T) {
			adapter := &fakeObservability{name: "pagerduty", active: true,
				metricsErr: fmt.Errorf("pagerduty: metrics: %w", observability.ErrUnsupported)}
			env := newObservabilityEnv(envObservability, adapter)

			res := measure(t, env, v.capID, map[string]any{"query": "q"})

			assertInconclusive(t, res, v, objective.ReasonUnsupported)
			if adapter.metricCalls != 1 {
				t.Errorf("the adapter was asked %d times, want once", adapter.metricCalls)
			}
		})
	}
}

// (e) An adapter error, or more than one series, is inconclusive:
// verifier_error. Which series the author meant is not guessed, and the error
// is not reported as a measurement.
func TestRuntimeVerifierErrorsAreVerifierErrors(t *testing.T) {
	cases := map[string]func(t *testing.T) *observabilityEnv{
		"prometheus 500": func(t *testing.T) *observabilityEnv {
			env, _ := prometheusEnv(t, http.StatusInternalServerError, `{"status":"error","error":"boom"}`)
			return env
		},
		"prometheus two series": func(t *testing.T) *observabilityEnv {
			env, _ := prometheusEnv(t, http.StatusOK, promTwoSeries)
			return env
		},
		"datadog 500": func(t *testing.T) *observabilityEnv {
			env, _ := datadogEnv(t, http.StatusInternalServerError, `{"errors":["boom"]}`)
			return env
		},
		"adapter error": func(t *testing.T) *observabilityEnv {
			return newObservabilityEnv(envObservability, &fakeObservability{name: "prometheus", active: true,
				metricsErr: errors.New("prometheus: metrics: connection refused")})
		},
	}
	for name, build := range cases {
		for _, v := range runtimeVerifiers {
			t.Run(name+"/"+v.metric, func(t *testing.T) {
				res := measure(t, build(t), v.capID, map[string]any{"query": "q"})
				assertInconclusive(t, res, v, objective.ReasonVerifierError)
			})
		}
	}
}

// (f) An environment that cannot see fails the action; it never reports a
// value, and never asks an adapter it does not have.
func TestRuntimeVerifierWithoutABoundInstanceFails(t *testing.T) {
	for _, v := range runtimeVerifiers {
		t.Run(v.metric+"/unbound", func(t *testing.T) {
			res := measure(t, newObservabilityEnv(envObservability, nil), v.capID, map[string]any{"query": "q"})
			if res.Success || len(res.Measurements) != 0 {
				t.Errorf("an unbound environment reported success %v with %+v", res.Success, res.Measurements)
			}
			if res.Error == "" {
				t.Error("the failure says nothing about why")
			}
		})
		t.Run(v.metric+"/inactive", func(t *testing.T) {
			adapter := &fakeObservability{name: "prometheus", active: false,
				series: []observability.MetricSeries{{Name: "x", Points: []observability.MetricPoint{{Time: time.Now(), Value: 1}}}}}
			res := measure(t, newObservabilityEnv(envObservability, adapter), v.capID, map[string]any{"query": "q"})
			if res.Success || len(res.Measurements) != 0 {
				t.Errorf("an inactive instance reported success %v with %+v", res.Success, res.Measurements)
			}
			if adapter.metricCalls != 0 {
				t.Error("an inactive instance was asked for metrics")
			}
		})
	}
}

// params.query is required: without it there is nothing to measure, and the
// backend is not asked.
func TestRuntimeVerifierRefusesWithoutAQuery(t *testing.T) {
	for _, v := range runtimeVerifiers {
		for name, params := range map[string]map[string]any{
			"no params":   nil,
			"empty query": {"query": ""},
			"not a query": {"since_minutes": 30},
		} {
			t.Run(v.metric+"/"+name, func(t *testing.T) {
				adapter := &fakeObservability{name: "prometheus", active: true,
					series: []observability.MetricSeries{{Name: "x", Points: []observability.MetricPoint{{Time: time.Now(), Value: 1}}}}}
				res := measure(t, newObservabilityEnv(envObservability, adapter), v.capID, params)
				if res.Success || len(res.Measurements) != 0 {
					t.Errorf("measured %+v without a query", res.Measurements)
				}
				if res.Error == "" {
					t.Error("the refusal does not say what is missing")
				}
				if adapter.metricCalls != 0 {
					t.Error("the backend was asked without a query")
				}
			})
		}
	}
}

// params.since_minutes sets the window, as for fetch_metrics: default 60.
func TestRuntimeVerifierReadsTheWindow(t *testing.T) {
	for _, v := range runtimeVerifiers {
		for name, tc := range map[string]struct {
			params  map[string]any
			minutes int
		}{
			"default":  {map[string]any{"query": "q"}, observabilityDefaultSinceMinutes},
			"explicit": {map[string]any{"query": "q", "since_minutes": float64(15)}, 15},
		} {
			t.Run(v.metric+"/"+name, func(t *testing.T) {
				adapter := &fakeObservability{name: "prometheus", active: true,
					series: []observability.MetricSeries{{Name: "x", Points: []observability.MetricPoint{{Time: time.Now(), Value: 0.5}}}}}
				before := time.Now().UTC()
				measure(t, newObservabilityEnv(envObservability, adapter), v.capID, tc.params)
				after := time.Now().UTC()
				if adapter.metricCalls != 1 {
					t.Fatalf("the adapter was asked %d times, want once", adapter.metricCalls)
				}
				if adapter.lastMetricQuery.Query != "q" {
					t.Errorf("query = %q, want q", adapter.lastMetricQuery.Query)
				}
				assertSince(t, adapter.lastMetricQuery.Since, before, after, tc.minutes)
			})
		}
	}
}

// (g) Each verifier declares its metric, unit and direction, and the
// parameters a criterion may pass it, so create-time validation can check a
// criterion without running anything.
func TestRuntimeVerifiersDeclareTheirMetric(t *testing.T) {
	byID := map[capability.CapabilityID]capability.Capability{}
	for _, c := range New().Capabilities() {
		byID[c.ID] = c
	}
	for _, v := range runtimeVerifiers {
		t.Run(v.metric, func(t *testing.T) {
			c, ok := byID[capability.CapabilityID(v.capID)]
			if !ok {
				t.Fatalf("the software pack does not declare %s", v.capID)
			}
			if !c.Verifiable {
				t.Error("a verifier is not marked verifiable")
			}
			m, ok := c.DeclaredMetric(v.metric)
			if !ok {
				t.Fatalf("%s declares no metric %s: %+v", v.capID, v.metric, c.Metrics)
			}
			if len(c.Metrics) != 1 {
				t.Errorf("%s declares %d metrics, want 1", v.capID, len(c.Metrics))
			}
			if m.Unit != v.unit || m.Direction != capability.DirectionLowerIsBetter {
				t.Errorf("%s is declared in %q, %q; want %q, %q", v.metric, m.Unit, m.Direction, v.unit, capability.DirectionLowerIsBetter)
			}
			params := map[string]capability.ParamType{}
			for _, p := range m.Params {
				params[p.Name] = p.Type
			}
			if params["query"] != capability.ParamString || params["since_minutes"] != capability.ParamInteger || len(params) != 2 {
				t.Errorf("params = %v, want query (string) and since_minutes (integer)", params)
			}
			if !containsString(c.InputSchema.Required, "query") {
				t.Errorf("input schema requires %v, want query", c.InputSchema.Required)
			}
		})
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
