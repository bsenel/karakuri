package twin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type CreateRequest struct {
	Name   string
	Kind   twin.Kind
	Domain string
	// OwnerID records the principal creating the twin, so ownership-scoped
	// policies have something to match on. Empty leaves the twin unowned.
	OwnerID string
}

type Service struct {
	store storage.StorageAdapter
	hub   *event.Hub
}

func NewService(store storage.StorageAdapter, hub *event.Hub) *Service {
	return &Service{store: store, hub: hub}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (twin.DigitalTwin, error) {
	id, _ := newID()
	t := twin.DigitalTwin{
		ID: id, Name: req.Name, Kind: req.Kind, Domain: req.Domain,
		OwnerID:   req.OwnerID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveTwin(ctx, t); err != nil {
		return twin.DigitalTwin{}, fmt.Errorf("save twin: %w", err)
	}
	s.hub.Publish(ctx, event.Event{
		Type: event.TypeTwinStateUpdated, TwinID: id,
		Payload:   map[string]any{"action": "created", "name": req.Name},
		Timestamp: time.Now().UTC(),
	})
	return t, nil
}

func (s *Service) Get(ctx context.Context, id string) (twin.DigitalTwin, error) {
	return s.store.GetTwin(ctx, id)
}

func (s *Service) List(ctx context.Context, f storage.TwinFilter) ([]twin.DigitalTwin, error) {
	return s.store.ListTwins(ctx, f)
}

func (s *Service) Update(ctx context.Context, t twin.DigitalTwin) error {
	t.UpdatedAt = time.Now().UTC()
	return s.store.UpdateTwin(ctx, t)
}

func newID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
