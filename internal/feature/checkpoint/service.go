package checkpoint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

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

func (s *Service) Create(ctx context.Context, objectiveID objective.ObjectiveID, twinID, reason, summary string, options []string) (corecheckpoint.Checkpoint, error) {
	id, _ := newID()
	cp := corecheckpoint.Checkpoint{
		ID: id, ObjectiveID: objectiveID, TwinID: twinID,
		Reason: reason, Summary: summary, Options: options,
		Status: corecheckpoint.StatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveCheckpoint(ctx, cp); err != nil {
		return corecheckpoint.Checkpoint{}, err
	}
	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeCheckpoint,
		ObjectiveID: string(objectiveID),
		TwinID:      twinID,
		Payload:     map[string]any{"id": id, "summary": summary, "options": options},
		Timestamp:   time.Now().UTC(),
	})
	return cp, nil
}

func (s *Service) Get(ctx context.Context, id string) (corecheckpoint.Checkpoint, error) {
	return s.store.GetCheckpoint(ctx, id)
}

func (s *Service) ListPending(ctx context.Context, twinID string) ([]corecheckpoint.Checkpoint, error) {
	return s.store.ListPendingCheckpoints(ctx, twinID)
}

func (s *Service) Resolve(ctx context.Context, id string, d corecheckpoint.Decision) error {
	return s.store.ResolveCheckpoint(ctx, id, d)
}

func newID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
