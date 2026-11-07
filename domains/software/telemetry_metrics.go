package software

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
)

// The telemetry verifiers (Phase 41 step 13): two numbers the deployment
// measures about itself, so a standing objective can hold a measured
// criterion over them. Each is read from one telemetry Snapshot and emitted
// as a single first-party measurement; whatever the window cannot answer is
// inconclusive, never a 0 or a -1 that would read as a value.

// telemetryQuery is the window and twin a telemetry read asks for, shared by
// analyse_usage and the two verifiers.
//
// params.window is a Go duration and falls back to telemetryWindow when it is
// missing, malformed or not positive. The environment's twin is a ceiling,
// not a default: a plan asking for another tenant's numbers, or for the whole
// deployment from inside one tenant, is asking for the cross-tenant read the
// telemetry reader was fixed to prevent, so narrowing is allowed and widening
// is not.
func (e *telemetryEnv) telemetryQuery(params map[string]any) (coretelemetry.Query, time.Duration) {
	window := telemetryWindow
	if s, ok := params["window"].(string); ok && s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			window = d
		}
	}
	twinID := e.twinID
	if s, ok := params["twin_id"].(string); ok && s != "" && e.twinID == "" {
		twinID = s
	}
	return coretelemetry.Query{Since: time.Now().UTC().Add(-window), TwinID: twinID}, window
}

// telemetryVerify measures one of the two telemetry verifiers. The second
// result is false when the capability is not one of them.
func (e *telemetryEnv) telemetryVerify(ctx context.Context, a environment.Action) (environment.ActionResult, bool) {
	verifier := string(a.CapabilityID)
	switch verifier {
	case CapCostPerPass, CapEscalationApprovalRate:
	default:
		return environment.ActionResult{}, false
	}
	// A deployment with no reader cannot see itself: unsupported, never an
	// empty success (AGENTS.md rule 10, ADR 026).
	if e.reader == nil {
		return inconclusive(verifier, objective.ReasonUnsupported, "no telemetry reader is wired into this deployment"), true
	}
	q, window := e.telemetryQuery(a.Params)
	snap, err := e.reader.Snapshot(ctx, q)
	if err != nil {
		return inconclusive(verifier, objective.ReasonVerifierError, fmt.Sprintf("could not read the deployment's telemetry: %v", err)), true
	}
	if verifier == CapCostPerPass {
		return costPerPass(snap, window), true
	}
	return escalationApprovalRate(snap, window), true
}

// costPerPass is Spend.Cost over Work.Reconciles, in USD.
//
// A pass is a reconcile outcome carrying a LoopID, counted into
// Work.Reconciles at internal/platform/telemetry/reader.go:113; an outcome
// without one is a sense, which spends no model call and is not a pass.
// Spend.Cost and Spend.Priced (internal/core/telemetry/telemetry.go:91-97)
// are set by readSpend at internal/platform/telemetry/reader.go:237-270:
// Priced stays false when the cost report could not be read (:243-246) and is
// cleared when units were counted at no cost, which is no rate table (:267-269).
func costPerPass(snap coretelemetry.Snapshot, window time.Duration) environment.ActionResult {
	if snap.Work.Reconciles == 0 {
		return inconclusive(CapCostPerPass, objective.ReasonNoData,
			fmt.Sprintf("no reconcile pass ran in the last %s, so there is nothing to divide the spend by", window))
	}
	if !snap.Spend.Priced {
		return inconclusive(CapCostPerPass, objective.ReasonNoData,
			fmt.Sprintf("the spend in the last %s is not priced: either no rate table prices the units counted, or the cost report was unread", window))
	}
	return telemetryMeasured(CapCostPerPass, "cost_per_pass", "usd",
		snap.Spend.Cost/float64(snap.Work.Reconciles), window)
}

// escalationApprovalRate is EscalationStats.ApprovalRate, as a ratio.
//
// Approvals and rejections are counted from audit tool events by readAudit at
// internal/platform/telemetry/reader.go:164-186, which counts 0 for a kind it
// could not list. A rejection is counted once, from the checkpoint service's
// row of the decision; the loop's terminal row for the same rejection, which
// carries an EscalationReason, is not counted. ApprovalRate (internal/core/telemetry/telemetry.go:83-89)
// returns -1 when nothing was resolved, and pending escalations are not
// resolved: "nobody decided" is no data, the opposite of "all rejected".
func escalationApprovalRate(snap coretelemetry.Snapshot, window time.Duration) environment.ActionResult {
	rate := snap.Escalation.ApprovalRate()
	if rate < 0 {
		return inconclusive(CapEscalationApprovalRate, objective.ReasonNoData,
			fmt.Sprintf("no escalation was approved or rejected in the last %s (%d pending)", window, snap.Escalation.Pending))
	}
	return telemetryMeasured(CapEscalationApprovalRate, "escalation_approval_rate", "ratio", rate, window)
}

// telemetryMeasured is a verifier that measured: one measurement of metric,
// first-party because the deployment wrote every number it is computed from.
// Whether it meets a criterion's target is the loop's to decide.
func telemetryMeasured(verifier, metric, unit string, value float64, window time.Duration) environment.ActionResult {
	return environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		Measurements: []environment.Measurement{{
			Name: metric, Value: value, Unit: unit,
			Source: "platform_telemetry:" + verifier,
			Trust:  environment.TrustOperator,
		}},
		StateDelta: map[string]any{
			"verifier": verifier,
			"metric":   metric,
			"value":    value,
			"unit":     unit,
			"window":   window.String(),
		},
	}
}

// telemetryMetricCapabilities declares the two verifiers. Their only param is
// the window; the twin is the environment's, so it is not a metric param.
func telemetryMetricCapabilities() []capability.Capability {
	input := capability.Schema{
		Type: "object",
		Properties: map[string]capability.SchemaProperty{
			"window": {Type: "string", Description: "How far back to measure, as a Go duration such as 24h. Defaults to a week"},
		},
	}
	params := []capability.MetricParam{{Name: "window", Type: capability.ParamString}}
	return []capability.Capability{
		{
			ID: CapCostPerPass, Name: "Cost Per Pass", Domain: "software",
			Description: "Measure this deployment's priced spend divided by its reconcile passes over the window, in USD, from its own telemetry. No pass, or spend that is not priced, is inconclusive: no_data, never 0.",
			Verifiable:  true,
			InputSchema: input,
			OutputSchema: capability.Schema{Type: "object", Properties: map[string]capability.SchemaProperty{
				"value": {Type: "number", Description: "USD per reconcile pass"},
			}},
			Metrics: []capability.Metric{{Name: "cost_per_pass", Unit: "usd", Direction: capability.DirectionLowerIsBetter, Params: params}},
		},
		{
			ID: CapEscalationApprovalRate, Name: "Escalation Approval Rate", Domain: "software",
			Description: "Measure the share of escalations resolved over the window that were approved, as a ratio from 0 to 1, from this deployment's own telemetry. Nothing resolved is inconclusive: no_data, never 0 or -1.",
			Verifiable:  true,
			InputSchema: input,
			OutputSchema: capability.Schema{Type: "object", Properties: map[string]capability.SchemaProperty{
				"value": {Type: "number", Description: "Approved over approved plus rejected"},
			}},
			Metrics: []capability.Metric{{Name: "escalation_approval_rate", Unit: "ratio", Direction: capability.DirectionHigherIsBetter, Params: params}},
		},
	}
}
