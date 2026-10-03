package software

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

const (
	CapFetchLogs    = "software.observe.fetch_logs"
	CapFetchMetrics = "software.observe.fetch_metrics"
)

const (
	observabilityDefaultSinceMinutes = 60
	observabilityMaxSinceMinutes     = 1440
	observabilityDefaultLogLimit     = 200
	observabilityDefaultStepSeconds  = 60
)

// observabilityEnv is the runtime as evidence: what is firing, and the logs and
// metrics behind it. adapter is nil when the twin binds no observability
// instance; there is no no-op adapter to fall back on, on purpose.
type observabilityEnv struct {
	id      environment.EnvironmentID
	adapter observability.ObservabilityAdapter
}

func newObservabilityEnv(id environment.EnvironmentID, adapter observability.ObservabilityAdapter) *observabilityEnv {
	return &observabilityEnv{id: id, adapter: adapter}
}

func (e *observabilityEnv) ID() environment.EnvironmentID { return e.id }
func (e *observabilityEnv) Domain() string                { return "software" }

// adapterName is the bound instance's name. Name() is on every shipped adapter
// but not on the exported interface, so it is asserted for here.
func (e *observabilityEnv) adapterName() string {
	if n, ok := e.adapter.(interface{ Name() string }); ok && n.Name() != "" {
		return n.Name()
	}
	return string(e.id)
}

// blindReason says why this environment cannot see, or "" when it can. Blind is
// not healthy: every caller turns a non-empty reason into a refusal or an
// error, never into an empty result.
func (e *observabilityEnv) blindReason() string {
	if e.adapter == nil {
		return "no observability instance is bound to this twin"
	}
	if !e.adapter.Active() {
		return fmt.Sprintf("observability instance %q is not active", e.adapterName())
	}
	return ""
}

// fetchOpenAlerts asks the adapter what is firing or acknowledged.
func (e *observabilityEnv) fetchOpenAlerts(ctx context.Context) ([]observability.Alert, error) {
	alerts, err := e.adapter.GetAlerts(ctx, "", "", time.Time{}, "")
	if err != nil {
		return nil, fmt.Errorf("%s: alerts: %w", e.id, err)
	}
	return openAlerts(alerts), nil
}

// openAlerts keeps the firing and acknowledged alerts, sorted by ID so the same
// set always serialises the same.
func openAlerts(alerts []observability.Alert) []observability.Alert {
	open := make([]observability.Alert, 0, len(alerts))
	for _, a := range alerts {
		if a.State == observability.AlertFiring || a.State == observability.AlertAcknowledged {
			open = append(open, a)
		}
	}
	sort.SliceStable(open, func(i, j int) bool { return open[i].ID < open[j].ID })
	return open
}

func alertStateCounts(alerts []observability.Alert) (firing, acknowledged int) {
	for _, a := range alerts {
		switch a.State {
		case observability.AlertFiring:
			firing++
		case observability.AlertAcknowledged:
			acknowledged++
		}
	}
	return firing, acknowledged
}

// severityRank orders severities for truncation, most severe first. Providers
// spell them differently; anything unrecognised sorts last.
func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "critical", "page", "p1", "sev1":
		return 0
	case "error", "high", "p2", "sev2":
		return 1
	case "warning", "warn", "medium", "p3", "sev3":
		return 2
	case "info", "low", "p4", "sev4":
		return 3
	}
	return 4
}

func (e *observabilityEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	if reason := e.blindReason(); reason != "" {
		return environment.Observation{}, fmt.Errorf("%s: %s", e.id, reason)
	}
	open, err := e.fetchOpenAlerts(ctx)
	if err != nil {
		return environment.Observation{}, err
	}
	firing, acknowledged := alertStateCounts(open)
	state := map[string]any{
		"adapter":      e.adapterName(),
		"firing":       firing,
		"acknowledged": acknowledged,
	}

	kept := open
	if q.Limit > 0 && len(open) > q.Limit {
		// The most severe survive the cut, then go back into ID order.
		kept = append([]observability.Alert(nil), open...)
		sort.SliceStable(kept, func(i, j int) bool {
			return severityRank(kept[i].Severity) < severityRank(kept[j].Severity)
		})
		kept = kept[:q.Limit]
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].ID < kept[j].ID })
		state["truncated"] = true
		state["total"] = len(open)
	}

	alerts := make([]map[string]any, 0, len(kept))
	for _, a := range kept {
		alerts = append(alerts, map[string]any{
			"id": a.ID, "service": a.Service, "severity": a.Severity,
			"state": a.State, "message": a.Message,
			"since": a.Time.UTC().Format(time.RFC3339),
		})
	}
	state["alerts"] = alerts

	// An alert's message is an annotation or an incident title somebody typed,
	// so one carried alert makes the observation third-party. An empty list is
	// nobody's writing and stays the operator's.
	trust := environment.TrustOperator
	if len(alerts) > 0 {
		trust = environment.TrustThirdParty
	}

	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state),
		Timestamp: time.Now().UTC(), Trust: trust,
	}, nil
}

func (e *observabilityEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	switch a.CapabilityID {
	case CapFetchLogs:
		return e.fetchLogs(ctx, a.Params), nil
	case CapFetchMetrics:
		return e.fetchMetrics(ctx, a.Params), nil
	}
	return observabilityRefusal("%s fetches logs and metrics; %s cannot be executed here", e.id, a.CapabilityID), nil
}

// observabilityRefusal is a look that did not happen: never Success true with
// an empty result for something that was not looked at.
func observabilityRefusal(format string, args ...any) environment.ActionResult {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf(format, args...)}
}

func (e *observabilityEnv) fetchLogs(ctx context.Context, params map[string]any) environment.ActionResult {
	if reason := e.blindReason(); reason != "" {
		return observabilityRefusal("%s: %s", CapFetchLogs, reason)
	}
	query, service := asString(params, "query"), asString(params, "service")
	if query == "" && service == "" {
		return observabilityRefusal("%s needs something to look for: pass params.query and/or params.service", CapFetchLogs)
	}

	lines, err := e.adapter.FetchLogs(ctx, observability.LogQuery{
		Service: service,
		Query:   query,
		Since:   sinceParam(params),
		Limit:   positiveIntParam(params, "limit", observabilityDefaultLogLimit),
	})
	if errors.Is(err, observability.ErrUnsupported) {
		return observabilityRefusal("%s: the type of observability instance %q does not support logs: %v", CapFetchLogs, e.adapterName(), err)
	}
	if err != nil {
		return observabilityRefusal("%s: %v", CapFetchLogs, err)
	}

	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{
			"time": l.Time.UTC().Format(time.RFC3339), "service": l.Service, "message": l.Message,
		})
	}

	// Log lines carry text from whatever wrote them, including users of the
	// watched system. A query that matched nothing carries nobody's.
	trust := environment.TrustOperator
	if len(out) > 0 {
		trust = environment.TrustThirdParty
	}
	return environment.ActionResult{Success: true, Trust: trust, StateDelta: map[string]any{
		"lines": out, "count": len(out),
	}}
}

func (e *observabilityEnv) fetchMetrics(ctx context.Context, params map[string]any) environment.ActionResult {
	if reason := e.blindReason(); reason != "" {
		return observabilityRefusal("%s: %s", CapFetchMetrics, reason)
	}
	query := asString(params, "query")
	if query == "" {
		return observabilityRefusal("%s needs a query: pass params.query", CapFetchMetrics)
	}

	series, err := e.adapter.FetchMetrics(ctx, observability.MetricQuery{
		Query: query,
		Since: sinceParam(params),
		Step:  time.Duration(positiveIntParam(params, "step_seconds", observabilityDefaultStepSeconds)) * time.Second,
	})
	if errors.Is(err, observability.ErrUnsupported) {
		return observabilityRefusal("%s: the type of observability instance %q does not support metrics: %v", CapFetchMetrics, e.adapterName(), err)
	}
	if err != nil {
		return observabilityRefusal("%s: %v", CapFetchMetrics, err)
	}

	out := make([]map[string]any, 0, len(series))
	for _, s := range series {
		points := make([]map[string]any, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, map[string]any{
				"time": p.Time.UTC().Format(time.RFC3339), "value": p.Value,
			})
		}
		out = append(out, map[string]any{"name": s.Name, "labels": s.Labels, "points": points})
	}

	// Label values and numbers are the operator's own infrastructure, not prose.
	return environment.ActionResult{Success: true, Trust: environment.TrustOperator, StateDelta: map[string]any{
		"series": out, "count": len(out),
	}}
}

// sinceParam turns params.since_minutes into the start of the window.
func sinceParam(params map[string]any) time.Time {
	minutes := positiveIntParam(params, "since_minutes", observabilityDefaultSinceMinutes)
	if minutes > observabilityMaxSinceMinutes {
		minutes = observabilityMaxSinceMinutes
	}
	return time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)
}

// positiveIntParam reads a numeric param, which arrives as float64 off JSON, as
// int from Go callers and as a string from models that quote numbers. Anything
// missing, unparsable or not positive is the default.
func positiveIntParam(params map[string]any, key string, def int) int {
	n := asInt(params, key)
	if s, ok := params[key].(string); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			n = int(f)
		}
	}
	if n <= 0 {
		return def
	}
	return n
}

func (e *observabilityEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

// Snapshot reports an empty SHA when nothing is bound or the instance is not
// active, and the error when the adapter could not be asked. It never returns
// a constant SHA: an environment that cannot see must not return a still
// snapshot, because Phase 20 will read that as a quiet world and stop looking.
func (e *observabilityEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	snap := environment.EnvironmentSnapshot{EnvID: e.id, Timestamp: time.Now().UTC()}
	if e.blindReason() != "" {
		return snap, nil // blind, and the supervisor reads it as blind
	}
	open, err := e.fetchOpenAlerts(ctx)
	if err != nil {
		return snap, err
	}
	firing, acknowledged := alertStateCounts(open)
	snap.SHA = observabilityFingerprint(open)
	snap.State = map[string]any{"firing": firing, "acknowledged": acknowledged}
	return snap, nil
}

// observabilityFingerprint hashes which alerts are open rather than what they
// currently say.
//
// ADR 017's rule is that the SHA answers "has anything changed that is worth
// waking up for". An alert starting, resolving or being acknowledged is; so is
// a severity's count crossing an order of magnitude. The same alerts still
// firing are not, and neither is a message being re-rendered with a new value
// — a hash over messages or timestamps would move on every evaluation interval
// and an incident objective would reconcile all day on one unchanged alert. So
// the open set is hashed exactly, as sorted IDs each with its state, the
// per-severity counts are bucketed, and messages, timestamps, values and raw
// counts are left out.
func observabilityFingerprint(alerts []observability.Alert) string {
	open := openAlerts(alerts)
	parts := make([]string, 0, len(open)+4)
	perSeverity := map[string]int{}
	for _, a := range open {
		parts = append(parts, "alert="+a.ID+":"+a.State)
		perSeverity[a.Severity]++
	}
	severities := make([]string, 0, len(perSeverity))
	for s := range perSeverity {
		severities = append(severities, s)
	}
	sort.Strings(severities)
	for _, s := range severities {
		parts = append(parts, "severity="+s+":"+bucket(perSeverity[s]))
	}
	return vfs.SHA([]byte(strings.Join(parts, "|")))
}
