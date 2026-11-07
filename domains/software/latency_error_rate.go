package software

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

// measureLatest is the runtime verifiers' shared body (Phase 41 step 12): it
// asks the bound backend params.query over params.since_minutes, the same call
// fetchMetrics makes, and reports the latest point of the one series returned
// as metric in unit.
//
// What it cannot measure is inconclusive with a reason, never a zero and never
// a measurement: no series, no points or a NaN is no_data; a backend without
// metrics is unsupported; an adapter error, or more than one series, which
// would mean guessing the one the author meant, is a verifier_error.
func (e *observabilityEnv) measureLatest(ctx context.Context, verifier, metric, unit string, params map[string]any) environment.ActionResult {
	if reason := e.blindReason(); reason != "" {
		return observabilityRefusal("%s: %s", verifier, reason)
	}
	query := asString(params, "query")
	if query == "" {
		return observabilityRefusal("%s needs a query: pass params.query", verifier)
	}

	series, err := e.adapter.FetchMetrics(ctx, observability.MetricQuery{
		Query: query,
		Since: sinceParam(params),
		Step:  time.Duration(observabilityDefaultStepSeconds) * time.Second,
	})
	if errors.Is(err, observability.ErrUnsupported) {
		return inconclusive(verifier, objective.ReasonUnsupported,
			fmt.Sprintf("the type of observability instance %q does not support metrics: %v", e.adapterName(), err))
	}
	if err != nil {
		return inconclusive(verifier, objective.ReasonVerifierError, fmt.Sprintf("could not query metrics: %v", err))
	}

	switch {
	case len(series) == 0:
		return inconclusive(verifier, objective.ReasonNoData, "the query returned no series")
	case len(series) > 1:
		return inconclusive(verifier, objective.ReasonVerifierError,
			fmt.Sprintf("the query returned %d series, want exactly one: aggregate it to one series", len(series)))
	}
	points := series[0].Points
	if len(points) == 0 {
		return inconclusive(verifier, objective.ReasonNoData, "the series has no points in the window")
	}
	latest := points[len(points)-1]
	if math.IsNaN(latest.Value) || math.IsInf(latest.Value, 0) {
		return inconclusive(verifier, objective.ReasonNoData, fmt.Sprintf("the latest point is %v", latest.Value))
	}

	// A number from this deployment's own bound backend, like fetchMetrics':
	// first-party, so it can settle a required criterion.
	return environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"verifier": verifier, "metric": metric, "value": latest.Value, "unit": unit,
			"time": latest.Time.UTC().Format(time.RFC3339),
		},
		Measurements: []environment.Measurement{{
			Name: metric, Value: latest.Value, Unit: unit,
			Source: fmt.Sprintf("%s:%s", e.adapterName(), verifier),
			Trust:  environment.TrustOperator,
		}},
	}
}
