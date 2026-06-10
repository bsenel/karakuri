package checkpoint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Service struct {
	store storage.StorageAdapter
	hub   *event.Hub
}

func NewService(store storage.StorageAdapter, hub *event.Hub) *Service {
	return &Service{store: store, hub: hub}
}

// CreateOptions carries the planner draft surfaced to reviewers when a
// loop escalates (Phase 13.5). All fields are optional — callers that
// don't have planner context (legacy code paths, manual checkpoints) pass
// a zero-value struct.
type CreateOptions struct {
	Capability   capability.CapabilityID
	Confidence   float64
	Actions      []corecheckpoint.Action
	AuditEventID string
}

// Create persists a pending checkpoint and publishes a checkpoint event.
// The CreateOptions block (Phase 13.5) carries the planner draft so the
// GET /api/v1/checkpoints/{id} response surfaces what the reviewer is
// approving. Existing callers that only have headline context pass a
// zero-value CreateOptions.
func (s *Service) Create(
	ctx context.Context,
	objectiveID objective.ObjectiveID,
	twinID, reason, summary string,
	options []string,
	opts CreateOptions,
) (corecheckpoint.Checkpoint, error) {
	id, _ := newID()
	cp := corecheckpoint.Checkpoint{
		ID: id, ObjectiveID: objectiveID, TwinID: twinID,
		Reason: reason, Summary: summary, Options: options,
		Capability:   opts.Capability,
		Confidence:   opts.Confidence,
		Actions:      opts.Actions,
		AuditEventID: opts.AuditEventID,
		Status:       corecheckpoint.StatusPending,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.SaveCheckpoint(ctx, cp); err != nil {
		return corecheckpoint.Checkpoint{}, err
	}
	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeCheckpoint,
		ObjectiveID: string(objectiveID),
		TwinID:      twinID,
		Payload: map[string]any{
			"id":             id,
			"summary":        summary,
			"options":        options,
			"capability":     string(opts.Capability),
			"confidence":     opts.Confidence,
			"action_count":   len(opts.Actions),
			"audit_event_id": opts.AuditEventID,
		},
		Timestamp: time.Now().UTC(),
	})
	return cp, nil
}

func (s *Service) Get(ctx context.Context, id string) (corecheckpoint.Checkpoint, error) {
	return s.store.GetCheckpoint(ctx, id)
}

func (s *Service) ListPending(ctx context.Context, twinID string) ([]corecheckpoint.Checkpoint, error) {
	return s.store.ListPendingCheckpoints(ctx, twinID)
}

// Resolve persists the operator decision and writes an audit row whose
// Kind matches the choice: "approval" for approve, "rejection" for
// reject, "modification" for modify. The modification payload carries
// the structured diff (removed_actions, added_constraints,
// revised_confidence) so the audit log shows *what* changed, not just
// *that* something changed (Phase 13.5).
func (s *Service) Resolve(ctx context.Context, id string, d corecheckpoint.Decision) error {
	if err := s.store.ResolveCheckpoint(ctx, id, d); err != nil {
		return err
	}
	cp, err := s.store.GetCheckpoint(ctx, id)
	if err != nil {
		// Resolve already succeeded — losing the audit record is regrettable
		// but not worth failing the request.
		return nil
	}

	kind := storage.ToolEventApproval
	success := true
	switch d.Choice {
	case "reject":
		kind = storage.ToolEventRejection
		success = false
	case "modify":
		kind = storage.ToolEventModification
		success = true
	}

	payload := map[string]any{
		"checkpoint_id":      id,
		"choice":             d.Choice,
		"note":               d.Note,
		"linked_audit_event": cp.AuditEventID,
	}
	if d.Modifications != nil {
		payload["modifications"] = d.Modifications
	}
	pj, _ := json.Marshal(payload)
	_ = s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		ObjectiveID: string(cp.ObjectiveID),
		Kind:        kind,
		Approver:    d.Approver,
		PayloadJSON: string(pj),
		Success:     success,
	})
	return nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
