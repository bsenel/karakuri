package software

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

const envObservability = environment.EnvironmentID("software.env.observability")

type fakeObservability struct {
	name   string
	active bool

	alerts     []observability.Alert
	alertsErr  error
	logs       []observability.LogLine
	logsErr    error
	series     []observability.MetricSeries
	metricsErr error

	alertCalls, logCalls, metricCalls int

	lastEnv, lastService, lastThreshold string
	lastSince                           time.Time
	lastLogQuery                        observability.LogQuery
	lastMetricQuery                     observability.MetricQuery
}

func (f *fakeObservability) Name() string { return f.name }
func (f *fakeObservability) Active() bool { return f.active }

func (f *fakeObservability) GetAlerts(_ context.Context, env, service string, since time.Time, threshold string) ([]observability.Alert, error) {
	f.alertCalls++
	f.lastEnv, f.lastService, f.lastSince, f.lastThreshold = env, service, since, threshold
	return append([]observability.Alert(nil), f.alerts...), f.alertsErr
}

func (f *fakeObservability) FetchLogs(_ context.Context, q observability.LogQuery) ([]observability.LogLine, error) {
	f.logCalls++
	f.lastLogQuery = q
	return f.logs, f.logsErr
}

func (f *fakeObservability) FetchMetrics(_ context.Context, q observability.MetricQuery) ([]observability.MetricSeries, error) {
	f.metricCalls++
	f.lastMetricQuery = q
	return f.series, f.metricsErr
}

func observabilityFactory(t *testing.T, reg *tools.Registry) environment.Factory {
	t.Helper()
	for _, f := range softwareEnvironmentFactories(reg) {
		if f.EnvID == envObservability {
			return f
		}
	}
	t.Fatalf("no factory for %s", envObservability)
	return environment.Factory{}
}

func obsAlert(id, severity, state string) observability.Alert {
	return observability.Alert{
		ID: id, Service: "checkout", Severity: severity, State: state,
		Message: "alert " + id, Time: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
}

func obsAlerts(n int, severity string) []observability.Alert {
	out := make([]observability.Alert, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, obsAlert(fmt.Sprintf("a-%03d", i), severity, observability.AlertFiring))
	}
	return out
}

func alertIDs(t *testing.T, state map[string]any) []string {
	t.Helper()
	alerts, ok := state["alerts"].([]map[string]any)
	if !ok {
		t.Fatalf("state[alerts] is %T, want []map[string]any", state["alerts"])
	}
	ids := make([]string, 0, len(alerts))
	for _, a := range alerts {
		ids = append(ids, fmt.Sprint(a["id"]))
	}
	return ids
}

// An environment that cannot see says so. The old factory answered
// {"status":"noop"} with the constant SHA "noop-snapshot", which reads to a
// standing objective as "looked, nothing changed" for ever.
func TestObservabilityUnboundIsBlindNotQuiet(t *testing.T) {
	const want = "no observability instance is bound to this twin"

	fromFactory := func(reg *tools.Registry) environment.Environment {
		env, err := observabilityFactory(t, reg).Build(environment.BuildContext{TwinID: "twin-1"})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}

	for _, tc := range []struct {
		name string
		env  environment.Environment
	}{
		{"nil adapter", newObservabilityEnv(envObservability, nil)},
		{"nil registry", fromFactory(nil)},
		{"nothing resolves", fromFactory(tools.NewRegistry())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.env.Observe(context.Background(), environment.ObservationQuery{})
			if err == nil {
				t.Fatal("observe succeeded with nothing bound")
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("observe error %q does not say %q", err, want)
			}

			snap, err := tc.env.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if snap.SHA == "noop-snapshot" {
				t.Error("snapshot still reports the constant noop SHA")
			}
			if snap.SHA != "" {
				t.Errorf("snapshot SHA = %q, want empty for an environment that cannot see", snap.SHA)
			}
		})
	}
}

func TestObservabilityInactiveAdapterIsBlind(t *testing.T) {
	adapter := &fakeObservability{name: "prod", active: false, alerts: obsAlerts(2, "critical")}
	env := newObservabilityEnv(envObservability, adapter)

	_, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err == nil {
		t.Fatal("observe succeeded on an inactive instance")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Errorf("observe error %q does not say the instance is not active", err)
	}

	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.SHA != "" {
		t.Errorf("snapshot SHA = %q, want empty", snap.SHA)
	}
}

// A failed look is not an empty one. Snapshot errors too: an empty SHA would
// drop the environment out of the composite fingerprint and read as no drift.
func TestObservabilityAlertErrorsSurface(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"plain", errors.New("prometheus: 502 bad gateway")},
		// A loki-only binding: logs, no alerts.
		{"unsupported", fmt.Errorf("loki: alerts: %w", observability.ErrUnsupported)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newObservabilityEnv(envObservability, &fakeObservability{name: "prod", active: true, alertsErr: tc.err})

			_, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err == nil {
				t.Fatal("observe succeeded although GetAlerts failed")
			}
			if !errors.Is(err, tc.err) && !strings.Contains(err.Error(), tc.err.Error()) {
				t.Errorf("observe error %q neither wraps nor names %q", err, tc.err)
			}

			snap, err := env.Snapshot(context.Background())
			if err == nil {
				t.Error("snapshot returned no error although GetAlerts failed")
			}
			if snap.SHA != "" {
				t.Errorf("snapshot SHA = %q, want empty on error", snap.SHA)
			}
		})
	}
}

func TestObservabilityResolvesTheTwinsBinding(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Observability.Set("prod", "fake", &fakeObservability{
		name: "prod", active: true, alerts: []observability.Alert{obsAlert("p-1", "critical", observability.AlertFiring)},
	})
	reg.Observability.Set("staging", "fake", &fakeObservability{
		name: "staging", active: true, alerts: []observability.Alert{
			obsAlert("s-1", "warning", observability.AlertFiring),
			obsAlert("s-2", "warning", observability.AlertFiring),
		},
	})
	factory := observabilityFactory(t, reg)

	for _, tc := range []struct {
		instance string
		want     []string
	}{
		{"prod", []string{"p-1"}},
		{"staging", []string{"s-1", "s-2"}},
	} {
		t.Run(tc.instance, func(t *testing.T) {
			env, err := factory.Build(environment.BuildContext{
				TwinID:          "twin-" + tc.instance,
				AdapterBindings: map[string]string{"observability": tc.instance},
			})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if obs.State["adapter"] != tc.instance {
				t.Errorf("adapter = %v, want %q", obs.State["adapter"], tc.instance)
			}
			if got := alertIDs(t, obs.State); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("alerts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestObservabilityObserveReportsAlerts(t *testing.T) {
	plus2 := time.FixedZone("plus2", 2*60*60)
	at := time.Date(2026, 10, 1, 14, 0, 0, 0, plus2)
	b := observability.Alert{ID: "b", Service: "checkout", Severity: "critical", State: observability.AlertFiring, Message: "5xx above 2%", Time: at}
	a := observability.Alert{ID: "a", Service: "search", Severity: "warning", State: observability.AlertAcknowledged, Message: "p99 slow", Time: at}
	c := observability.Alert{ID: "c", Service: "cart", Severity: "info", State: observability.AlertFiring, Message: "queue depth", Time: at}

	adapter := &fakeObservability{name: "prod", active: true, alerts: []observability.Alert{b, c, a}}
	env := newObservabilityEnv(envObservability, adapter)

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if adapter.alertCalls != 1 {
		t.Errorf("GetAlerts called %d times, want 1", adapter.alertCalls)
	}
	if adapter.lastEnv != "" || adapter.lastService != "" || adapter.lastThreshold != "" || !adapter.lastSince.IsZero() {
		t.Errorf("GetAlerts(env=%q, service=%q, since=%v, threshold=%q), want all empty and a zero since",
			adapter.lastEnv, adapter.lastService, adapter.lastSince, adapter.lastThreshold)
	}

	if obs.EnvID != envObservability {
		t.Errorf("EnvID = %q", obs.EnvID)
	}
	if obs.State["adapter"] != "prod" {
		t.Errorf("adapter = %v, want prod", obs.State["adapter"])
	}
	if obs.State["firing"] != 2 {
		t.Errorf("firing = %v, want 2", obs.State["firing"])
	}
	if obs.State["acknowledged"] != 1 {
		t.Errorf("acknowledged = %v, want 1", obs.State["acknowledged"])
	}
	if _, ok := obs.State["truncated"]; ok {
		t.Error("truncated is set although nothing was cut")
	}

	if got := alertIDs(t, obs.State); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("alerts in order %v, want sorted by ID", got)
	}
	first := obs.State["alerts"].([]map[string]any)[0]
	want := map[string]any{
		"id": "a", "service": "search", "severity": "warning",
		"state": observability.AlertAcknowledged, "message": "p99 slow",
		"since": "2026-10-01T12:00:00Z",
	}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("alert = %v, want %v", first, want)
	}

	if obs.Version == "" {
		t.Error("Version is empty")
	}
	if obs.Version != stateVersion(obs.State) {
		t.Errorf("Version = %q, want stateVersion(State) = %q", obs.Version, stateVersion(obs.State))
	}

	// The same set in another order is the same observation.
	reordered := newObservabilityEnv(envObservability, &fakeObservability{name: "prod", active: true, alerts: []observability.Alert{a, b, c}})
	again, err := reordered.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if fmt.Sprint(again.State["alerts"]) != fmt.Sprint(obs.State["alerts"]) {
		t.Errorf("alert order follows the adapter:\n%v\n%v", obs.State["alerts"], again.State["alerts"])
	}
	if again.Version != obs.Version {
		t.Error("Version depends on the order the adapter returned alerts in")
	}
}

func TestObservabilityObserveTruncatesBySeverity(t *testing.T) {
	adapter := &fakeObservability{name: "prod", active: true, alerts: []observability.Alert{
		obsAlert("a", "info", observability.AlertFiring),
		obsAlert("b", "critical", observability.AlertFiring),
		obsAlert("c", "warning", observability.AlertFiring),
	}}
	env := newObservabilityEnv(envObservability, adapter)

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{Limit: 2})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	got := map[string]bool{}
	for _, id := range alertIDs(t, obs.State) {
		got[id] = true
	}
	if len(got) != 2 || !got["b"] || !got["c"] {
		t.Errorf("kept %v, want the critical and the warning alert", got)
	}
	if obs.State["truncated"] != true {
		t.Errorf("truncated = %v, want true", obs.State["truncated"])
	}
	if obs.State["total"] != 3 {
		t.Errorf("total = %v, want 3", obs.State["total"])
	}
}

// An alert's message is somebody else's writing; an empty list is nobody's.
func TestObservabilityObserveTrust(t *testing.T) {
	for _, tc := range []struct {
		name   string
		alerts []observability.Alert
		want   environment.Trust
	}{
		{"alerts carried", obsAlerts(1, "critical"), environment.TrustThirdParty},
		{"no alerts", nil, environment.TrustOperator},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newObservabilityEnv(envObservability, &fakeObservability{name: "prod", active: true, alerts: tc.alerts})
			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if obs.Trust != tc.want {
				t.Errorf("Trust = %v, want %v", obs.Trust, tc.want)
			}
		})
	}
}

func observabilitySHA(t *testing.T, alerts ...observability.Alert) environment.EnvironmentSnapshot {
	t.Helper()
	env := newObservabilityEnv(envObservability, &fakeObservability{name: "prod", active: true, alerts: alerts})
	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.SHA == "" {
		t.Fatal("snapshot SHA is empty although the environment can see")
	}
	return snap
}

// The SHA is deliberately lossy: it moves when the set of open alerts moves and
// holds still while one alert re-words itself or refreshes its timestamp.
func TestObservabilitySnapshotSHA(t *testing.T) {
	a := obsAlert("a", "critical", observability.AlertFiring)
	b := obsAlert("b", "warning", observability.AlertFiring)
	base := observabilitySHA(t, a, b)

	if got := base.State["firing"]; got != 2 {
		t.Errorf("State[firing] = %v, want 2", got)
	}
	if got := base.State["acknowledged"]; got != 0 {
		t.Errorf("State[acknowledged] = %v, want 0", got)
	}

	reworded := a
	reworded.Message = "5xx at 7.31% over the last 5m"
	reworded.Time = a.Time.Add(3 * time.Hour)
	if got := observabilitySHA(t, reworded, b); got.SHA != base.SHA {
		t.Error("SHA moved when only a message and a timestamp changed")
	}

	if got := observabilitySHA(t, a, b, obsAlert("c", "warning", observability.AlertFiring)); got.SHA == base.SHA {
		t.Error("SHA did not move when an alert was added")
	}

	resolved := b
	resolved.State = observability.AlertResolved
	if got := observabilitySHA(t, a, resolved); got.SHA == base.SHA {
		t.Error("SHA did not move when an alert resolved")
	}
	if got := observabilitySHA(t, a); got.SHA == base.SHA {
		t.Error("SHA did not move when a resolved alert dropped out")
	}

	acked := b
	acked.State = observability.AlertAcknowledged
	got := observabilitySHA(t, a, acked)
	if got.SHA == base.SHA {
		t.Error("SHA did not move when an alert was acknowledged")
	}
	if got.State["firing"] != 1 || got.State["acknowledged"] != 1 {
		t.Errorf("State = %v, want firing 1 and acknowledged 1", got.State)
	}

	// Seeing nothing is still seeing: a non-empty SHA, and not the same one.
	if empty := observabilitySHA(t); empty.SHA == base.SHA {
		t.Error("zero alerts and two alerts share a SHA")
	}
}

// What is exact and what is bucketed, precisely.
//
// The alert SET — IDs and states — is hashed exactly, so adding an alert always
// moves the fingerprint: 41 and 42 alerts of one severity differ. What is
// bucketed is the per-severity COUNT component, and bucket puts 41 and 42 in
// the same band, so the count cannot be what moved it. Messages and timestamps
// are in neither component.
func TestObservabilityFingerprint(t *testing.T) {
	base := []observability.Alert{
		obsAlert("a", "critical", observability.AlertFiring),
		obsAlert("b", "warning", observability.AlertAcknowledged),
	}
	reworded := make([]observability.Alert, len(base))
	for i, al := range base {
		al.Message = "reworded " + al.ID
		al.Time = al.Time.Add(time.Duration(i+1) * time.Hour)
		reworded[i] = al
	}

	fp := observabilityFingerprint(base)
	if fp == "" {
		t.Fatal("fingerprint is empty")
	}
	if observabilityFingerprint(nil) == "" {
		t.Error("fingerprint of zero alerts is empty")
	}
	if observabilityFingerprint(nil) == fp {
		t.Error("fingerprint is constant across different alert sets")
	}
	if got := observabilityFingerprint(reworded); got != fp {
		t.Error("fingerprint moved for the same IDs and states with different messages and timestamps")
	}
	if got := observabilityFingerprint([]observability.Alert{base[1], base[0]}); got != fp {
		t.Error("fingerprint depends on alert order")
	}

	if bucket(41) != bucket(42) {
		t.Fatalf("bucket(41) = %q, bucket(42) = %q: the count component would move", bucket(41), bucket(42))
	}
	if bucket(9) == bucket(10) {
		t.Fatalf("bucket(9) and bucket(10) are both %q: bucket has no boundary there", bucket(9))
	}
	if observabilityFingerprint(obsAlerts(41, "warning")) == observabilityFingerprint(obsAlerts(42, "warning")) {
		t.Error("41 and 42 alerts share a fingerprint: the alert set is not hashed exactly")
	}
}

func TestObservabilityFetchLogs(t *testing.T) {
	lines := []observability.LogLine{
		{Time: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), Service: "checkout", Message: "payment timeout"},
		{Time: time.Date(2026, 10, 1, 12, 0, 1, 0, time.UTC), Service: "checkout", Message: "retrying"},
	}

	t.Run("needs query or service", func(t *testing.T) {
		adapter := &fakeObservability{name: "prod", active: true, logs: lines}
		res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
			CapabilityID: CapFetchLogs, Params: map[string]any{"limit": 10},
		})
		if err != nil {
			t.Fatalf("act: %v", err)
		}
		if res.Success {
			t.Error("fetch_logs succeeded with neither query nor service")
		}
		if !strings.Contains(res.Error, "query") || !strings.Contains(res.Error, "service") {
			t.Errorf("error %q does not name query and service", res.Error)
		}
		if adapter.logCalls != 0 {
			t.Errorf("FetchLogs called %d times, want 0", adapter.logCalls)
		}
	})

	for _, tc := range []struct {
		name        string
		params      map[string]any
		wantQuery   string
		wantService string
		wantMinutes int
		wantLimit   int
	}{
		{"defaults", map[string]any{"query": "timeout"}, "timeout", "", 60, 200},
		{"service only", map[string]any{"service": "checkout"}, "", "checkout", 60, 200},
		{"explicit", map[string]any{"query": "timeout", "service": "checkout", "since_minutes": 15, "limit": 25}, "timeout", "checkout", 15, 25},
		{"clamped", map[string]any{"query": "timeout", "since_minutes": 5000}, "timeout", "", 1440, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &fakeObservability{name: "prod", active: true, logs: lines}
			before := time.Now()
			res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
				CapabilityID: CapFetchLogs, Params: tc.params,
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if !res.Success {
				t.Fatalf("fetch_logs refused: %s", res.Error)
			}
			if adapter.logCalls != 1 {
				t.Fatalf("FetchLogs called %d times, want 1", adapter.logCalls)
			}
			q := adapter.lastLogQuery
			if q.Query != tc.wantQuery || q.Service != tc.wantService {
				t.Errorf("LogQuery{Query: %q, Service: %q}, want %q and %q", q.Query, q.Service, tc.wantQuery, tc.wantService)
			}
			if q.Limit != tc.wantLimit {
				t.Errorf("LogQuery.Limit = %d, want %d", q.Limit, tc.wantLimit)
			}
			assertSince(t, q.Since, before, after, tc.wantMinutes)

			got, ok := res.StateDelta["lines"].([]map[string]any)
			if !ok {
				t.Fatalf("StateDelta[lines] is %T, want []map[string]any", res.StateDelta["lines"])
			}
			if len(got) != 2 || res.StateDelta["count"] != 2 {
				t.Errorf("lines = %d, count = %v, want 2 and 2", len(got), res.StateDelta["count"])
			}
			if res.Trust != environment.TrustThirdParty {
				t.Errorf("Trust = %v, want third party when log lines are carried", res.Trust)
			}
		})
	}

	t.Run("no lines is trusted", func(t *testing.T) {
		adapter := &fakeObservability{name: "prod", active: true}
		res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
			CapabilityID: CapFetchLogs, Params: map[string]any{"query": "timeout"},
		})
		if err != nil {
			t.Fatalf("act: %v", err)
		}
		if !res.Success {
			t.Fatalf("fetch_logs refused: %s", res.Error)
		}
		if res.StateDelta["count"] != 0 {
			t.Errorf("count = %v, want 0", res.StateDelta["count"])
		}
		if res.Trust != environment.TrustOperator {
			t.Errorf("Trust = %v, want operator when nothing was returned", res.Trust)
		}
	})
}

// assertSince checks since is minutes before the moment Act ran.
func assertSince(t *testing.T, since, before, after time.Time, minutes int) {
	t.Helper()
	window := time.Duration(minutes) * time.Minute
	const slack = 5 * time.Second
	if since.Before(before.Add(-window-slack)) || since.After(after.Add(-window+slack)) {
		t.Errorf("Since = %v, want about %d minutes before %v", since, minutes, after)
	}
}

func TestObservabilityFetchMetrics(t *testing.T) {
	series := []observability.MetricSeries{{
		Name: "http_requests_total", Labels: map[string]string{"service": "checkout"},
		Points: []observability.MetricPoint{{Time: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), Value: 12.5}},
	}}

	t.Run("needs query", func(t *testing.T) {
		adapter := &fakeObservability{name: "prod", active: true, series: series}
		res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
			CapabilityID: CapFetchMetrics, Params: map[string]any{"since_minutes": 5},
		})
		if err != nil {
			t.Fatalf("act: %v", err)
		}
		if res.Success {
			t.Error("fetch_metrics succeeded with no query")
		}
		if !strings.Contains(res.Error, "query") {
			t.Errorf("error %q does not name query", res.Error)
		}
		if adapter.metricCalls != 0 {
			t.Errorf("FetchMetrics called %d times, want 0", adapter.metricCalls)
		}
	})

	for _, tc := range []struct {
		name        string
		params      map[string]any
		wantMinutes int
		wantStep    time.Duration
	}{
		{"defaults", map[string]any{"query": "rate(http_requests_total[5m])"}, 60, 60 * time.Second},
		{"explicit", map[string]any{"query": "rate(http_requests_total[5m])", "since_minutes": 30, "step_seconds": 15}, 30, 15 * time.Second},
		{"clamped", map[string]any{"query": "rate(http_requests_total[5m])", "since_minutes": 5000}, 1440, 60 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &fakeObservability{name: "prod", active: true, series: series}
			before := time.Now()
			res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
				CapabilityID: CapFetchMetrics, Params: tc.params,
			})
			after := time.Now()
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if !res.Success {
				t.Fatalf("fetch_metrics refused: %s", res.Error)
			}
			if adapter.metricCalls != 1 {
				t.Fatalf("FetchMetrics called %d times, want 1", adapter.metricCalls)
			}
			q := adapter.lastMetricQuery
			if q.Query != "rate(http_requests_total[5m])" {
				t.Errorf("MetricQuery.Query = %q", q.Query)
			}
			if q.Step != tc.wantStep {
				t.Errorf("MetricQuery.Step = %v, want %v", q.Step, tc.wantStep)
			}
			assertSince(t, q.Since, before, after, tc.wantMinutes)

			got, ok := res.StateDelta["series"].([]map[string]any)
			if !ok {
				t.Fatalf("StateDelta[series] is %T, want []map[string]any", res.StateDelta["series"])
			}
			if len(got) != 1 || res.StateDelta["count"] != 1 {
				t.Errorf("series = %d, count = %v, want 1 and 1", len(got), res.StateDelta["count"])
			}
			// Numbers and label sets, not prose.
			if res.Trust != environment.TrustOperator {
				t.Errorf("Trust = %v, want operator", res.Trust)
			}
		})
	}
}

// Never Success true for something that was not looked at.
func TestObservabilityActFailureModes(t *testing.T) {
	logsParams := map[string]any{"query": "timeout"}
	metricsParams := map[string]any{"query": "up"}
	boom := errors.New("datadog: 429 rate limited")
	unsupported := fmt.Errorf("pagerduty: %w", observability.ErrUnsupported)

	for _, tc := range []struct {
		name    string
		adapter observability.ObservabilityAdapter
		capID   capability.CapabilityID
		params  map[string]any
		want    []string
	}{
		{"logs unbound", nil, CapFetchLogs, logsParams, []string{"no observability instance is bound to this twin"}},
		{"metrics unbound", nil, CapFetchMetrics, metricsParams, []string{"no observability instance is bound to this twin"}},
		{"logs inactive", &fakeObservability{name: "prod"}, CapFetchLogs, logsParams, []string{"not active"}},
		{"metrics inactive", &fakeObservability{name: "prod"}, CapFetchMetrics, metricsParams, []string{"not active"}},
		{"logs unsupported", &fakeObservability{name: "prod", active: true, logsErr: unsupported}, CapFetchLogs, logsParams, []string{"logs", "support"}},
		{"metrics unsupported", &fakeObservability{name: "prod", active: true, metricsErr: unsupported}, CapFetchMetrics, metricsParams, []string{"metrics", "support"}},
		{"logs error", &fakeObservability{name: "prod", active: true, logsErr: boom}, CapFetchLogs, logsParams, []string{boom.Error()}},
		{"metrics error", &fakeObservability{name: "prod", active: true, metricsErr: boom}, CapFetchMetrics, metricsParams, []string{boom.Error()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A nil *fakeObservability in the interface would not be a nil
			// adapter, so the unbound cases pass an untyped nil.
			env := newObservabilityEnv(envObservability, tc.adapter)
			res, err := env.Act(context.Background(), environment.Action{CapabilityID: tc.capID, Params: tc.params})
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if res.Success {
				t.Fatal("reported success for something that was not looked at")
			}
			for _, w := range tc.want {
				if !strings.Contains(res.Error, w) {
					t.Errorf("error %q does not contain %q", res.Error, w)
				}
			}
			if f, ok := tc.adapter.(*fakeObservability); ok && !f.active && f.logCalls+f.metricCalls != 0 {
				t.Error("an inactive adapter was called")
			}
		})
	}
}

func TestObservabilityRefusesOtherCapabilities(t *testing.T) {
	adapter := &fakeObservability{name: "prod", active: true}
	res, err := newObservabilityEnv(envObservability, adapter).Act(context.Background(), environment.Action{
		CapabilityID: "software.act.write_code", Params: map[string]any{"query": "timeout"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Fatal("the observability environment accepted write_code")
	}
	if !strings.Contains(res.Error, "software.act.write_code") {
		t.Errorf("error %q does not name the refused capability", res.Error)
	}
	if adapter.alertCalls+adapter.logCalls+adapter.metricCalls != 0 {
		t.Error("the adapter was called for a capability this environment does not serve")
	}
}

// ADR 019 decision 4: which environment runs a capability is declared on the
// factory, and the registry routes on that.
func TestObservabilityServesLogsAndMetrics(t *testing.T) {
	want := []capability.CapabilityID{CapFetchLogs, CapFetchMetrics}
	if got := observabilityFactory(t, nil).Serves; !reflect.DeepEqual(got, want) {
		t.Errorf("Serves = %v, want %v", got, want)
	}

	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	for _, capID := range want {
		if got := reg.ServedBy(capID); !reflect.DeepEqual(got, []environment.EnvironmentID{envObservability}) {
			t.Errorf("%s is served by %v, want only %s", capID, got, envObservability)
		}
	}
}

func TestObservabilitySubscribeHasNoEvents(t *testing.T) {
	env := newObservabilityEnv(envObservability, &fakeObservability{name: "prod", active: true, alerts: obsAlerts(1, "critical")})
	ch, err := env.Subscribe(context.Background(), environment.EventFilter{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	select {
	case ev, ok := <-ch:
		if ok {
			t.Errorf("received an event: %+v", ev)
		}
	default:
	}
}
