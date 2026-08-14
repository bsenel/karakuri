package event

import (
	"context"
	"sync"
	"time"
)

// SSE event type constants.
const (
	TypeLoopStepStarted    = "loop_step_started"
	TypeLoopStepCompleted  = "loop_step_completed"
	TypeLoopIterationDone  = "loop_iteration_done"
	TypeObjectiveCompleted = "objective_completed"
	TypeObjectiveFailed    = "objective_failed"
	TypeObjectiveBlocked   = "objective_blocked"
	TypeCheckpoint         = "checkpoint"
	TypeAuthorityExceeded  = "authority_exceeded"
	// TypeQuotaPressure fires when a limit is most of the way used, so an
	// operator learns about a ceiling before it starts refusing work.
	TypeQuotaPressure      = "quota_pressure"
	TypeCostRecorded       = "cost_recorded"
	TypeEnvironmentChanged = "environment_changed"
	TypeMemoryRecalled     = "memory_recalled"
	TypeMemoryLearned      = "memory_learned"
	TypeWorktreeCreated    = "worktree_created"
	TypeWorktreeRemoved    = "worktree_removed"
	TypeArtifactWritten    = "artifact_written"
	TypeAdapterSkipped     = "adapter_skipped"
	TypeTwinStateUpdated   = "twin_state_updated"

	// The outer control loop over standing objectives (Phase 20). Every one
	// of these carries ObjectiveID and TwinID, which is what lets the global
	// stream decide who may see it — an event that named neither would be
	// unclassifiable and withheld from everybody.

	// TypeReconcileSensed is the cheap tier reporting. It fires far more
	// often than anything else here, and its whole job is to say that
	// nothing happened and nothing was spent.
	TypeReconcileSensed = "reconcile_sensed"
	// TypeDriftDetected is the world no longer matching the fingerprint
	// taken when the objective last converged.
	TypeDriftDetected = "drift_detected"
	// TypeReconcileStarted is an expensive pass beginning, carrying the
	// trigger that justified it.
	TypeReconcileStarted = "reconcile_started"
	// TypeConverged is desired state and actual state agreeing again.
	TypeConverged = "converged"
	// TypeReconcileFailed is a pass that errored. Escalation is not failure
	// and does not fire this.
	TypeReconcileFailed = "reconcile_failed"
	// TypeObjectiveSuspended is the circuit breaker or the stall detector
	// taking an objective out of rotation until somebody looks at it.
	TypeObjectiveSuspended = "objective_suspended"
	// TypeAutonomyChanged is a standing objective earning or losing a rung.
	// It is published separately from the reconcile that caused it because a
	// change in what Karakuri may do to the world is worth its own line.
	TypeAutonomyChanged = "autonomy_changed"
)

type Event struct {
	Type        string         `json:"type"`
	ObjectiveID string         `json:"objective_id,omitempty"`
	TwinID      string         `json:"twin_id,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Timestamp   time.Time      `json:"ts"`
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[string]map[chan Event]struct{})}
}

func (h *Hub) Publish(_ context.Context, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	keys := []string{"_global"}
	if evt.ObjectiveID != "" {
		keys = append(keys, "obj:"+evt.ObjectiveID)
	}
	if evt.TwinID != "" {
		keys = append(keys, "twin:"+evt.TwinID)
	}
	for _, key := range keys {
		for ch := range h.subscribers[key] {
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

// Subscribe returns a channel for events scoped to a key (e.g. "obj:<id>", "twin:<id>", "_global").
func (h *Hub) Subscribe(_ context.Context, key string) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	if h.subscribers[key] == nil {
		h.subscribers[key] = make(map[chan Event]struct{})
	}
	h.subscribers[key][ch] = struct{}{}
	h.mu.Unlock()
	unsub := func() {
		h.mu.Lock()
		delete(h.subscribers[key], ch)
		close(ch)
		h.mu.Unlock()
	}
	return ch, unsub
}
