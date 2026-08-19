// Package telemetry is the read-only port through which Karakuri can observe
// itself.
//
// It exists so a domain pack can treat this deployment's own behaviour as an
// environment — how often it escalates, what it spends, which objectives keep
// failing — without reaching into the storage adapter and without
// internal/core learning that such a pack exists. The interface is defined
// here and implemented in internal/platform/telemetry; packs receive it on
// environment.BuildContext, and it is nil for every pack that does not want it.
//
// Read-only is the whole design. A pack that could write here could rewrite
// the evidence of what it did, and the point of letting Karakuri watch itself
// is that the watching is trustworthy.
package telemetry

import (
	"context"
	"time"
)

// Reader answers what this deployment has been doing.
type Reader interface {
	Snapshot(ctx context.Context, q Query) (Snapshot, error)
}

// Query narrows a snapshot. An empty TwinID means the whole deployment, which
// is what a maintainer objective wants and what a tenant-scoped one must not
// have — the caller building the query is the one that knows which.
type Query struct {
	Since  time.Time
	TwinID string
}

// Snapshot is what Karakuri looks like to itself over a window.
type Snapshot struct {
	Since   time.Time `json:"since"`
	TakenAt time.Time `json:"taken_at"`

	Objectives ObjectiveStats  `json:"objectives"`
	Work       WorkStats       `json:"work"`
	Escalation EscalationStats `json:"escalation"`
	Spend      SpendStats      `json:"spend"`

	// Bottlenecks names what is going wrong most, already ranked. A pack
	// asking "what should I improve" should not have to derive this from
	// four counters and risk deriving it differently each time.
	Bottlenecks []Bottleneck `json:"bottlenecks,omitempty"`
}

type ObjectiveStats struct {
	Total     int `json:"total"`
	Standing  int `json:"standing"`
	Converged int `json:"converged"`
	Blocked   int `json:"blocked"`
	Failed    int `json:"failed"`
}

type WorkStats struct {
	// Senses and Reconciles are the two tiers of the outer loop. Their ratio
	// is the clearest single measure of whether the cheap tier is doing its
	// job.
	Senses     int `json:"senses"`
	Reconciles int `json:"reconciles"`
	Actions    int `json:"actions"`
	Failures   int `json:"failures"`
}

type EscalationStats struct {
	Escalations int `json:"escalations"`
	Approvals   int `json:"approvals"`
	Rejections  int `json:"rejections"`
	// Pending is how many decisions are outstanding right now rather than
	// over the window: a queue is a present-tense fact.
	Pending int `json:"pending"`
}

// ApprovalRate is the share of resolved escalations that were approved, and -1
// when nothing was resolved.
//
// Minus one rather than zero, because "nobody rejected anything" and "nobody
// decided anything" are opposite signals and a system reasoning about its own
// trustworthiness must not confuse them.
func (e EscalationStats) ApprovalRate() float64 {
	resolved := e.Approvals + e.Rejections
	if resolved == 0 {
		return -1
	}
	return float64(e.Approvals) / float64(resolved)
}

type SpendStats struct {
	Cost       float64            `json:"cost"`
	ByProvider map[string]float64 `json:"by_provider,omitempty"`
	// Priced is false when no rate table is configured, so a zero above means
	// "not priced" rather than "nothing was spent".
	Priced bool `json:"priced"`
}

// Bottleneck is something going wrong, named and counted.
type Bottleneck struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Count  int    `json:"count"`
}

// Bottleneck kinds. Kept as constants because a pack matching on them is
// writing against a contract, not against whatever string happened to be
// produced.
const (
	// BottleneckFailingObjective is an objective whose reconciles keep
	// erroring.
	BottleneckFailingObjective = "failing_objective"
	// BottleneckBlockedObjective is one the circuit breaker or the stall
	// detector has taken out of rotation.
	BottleneckBlockedObjective = "blocked_objective"
	// BottleneckStaleDecision is a checkpoint nobody has answered. The most
	// common way an autonomous system stops being autonomous is a queue of
	// questions nobody is reading.
	BottleneckStaleDecision = "stale_decision"
	// BottleneckFailingCapability is a capability failing across objectives,
	// which points at an adapter rather than at any one objective.
	BottleneckFailingCapability = "failing_capability"
)
