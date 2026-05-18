package checkpoint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Service struct {
	store  storage.StorageAdapter
	events *event.Hub
}

func NewService(store storage.StorageAdapter, events *event.Hub) *Service {
	return &Service{store: store, events: events}
}

func (s *Service) Create(ctx context.Context, sessionSHA, summary string, options []string) (entity.Checkpoint, error) {
	id, _ := newID()
	cp := entity.Checkpoint{
		ID: id, SessionSHA: sessionSHA, Summary: summary,
		Options: options, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveCheckpoint(ctx, cp); err != nil {
		return entity.Checkpoint{}, err
	}
	opts, _ := json.Marshal(options)
	_ = opts
	_ = s.events.Publish(ctx, event.Event{
		Type: event.Checkpoint, SessionSHA: sessionSHA,
		Payload: map[string]any{"id": id, "summary": summary, "options": options},
		Timestamp: time.Now().UTC(),
	})
	return cp, nil
}

func (s *Service) List(ctx context.Context, sessionSHA string) ([]entity.Checkpoint, error) {
	return s.store.ListCheckpoints(ctx, sessionSHA)
}

func (s *Service) Resolve(ctx context.Context, id string, decision entity.CheckpointDecision) error {
	return s.store.ResolveCheckpoint(ctx, id, decision)
}

func newID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
