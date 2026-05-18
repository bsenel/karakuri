package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Service struct {
	store storage.StorageAdapter
}

func NewService(store storage.StorageAdapter) *Service {
	return &Service{store: store}
}

type CreateRequest struct {
	Mode      entity.SessionMode
	Input     string
	ParentSHA string
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (entity.Session, error) {
	sha, err := newSHA()
	if err != nil {
		return entity.Session{}, err
	}
	now := time.Now().UTC()
	sess := entity.Session{
		SHA: sha, Mode: req.Mode, State: entity.StateCreated,
		ParentSHA: req.ParentSHA, Input: req.Input, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.SaveSession(ctx, sess); err != nil {
		return entity.Session{}, err
	}
	manifest := vfs.Manifest{
		SessionSHA: sha, Mode: string(req.Mode), ParentSHA: req.ParentSHA,
		Artifacts: make(map[string]string), Metadata: make(map[string]any),
	}
	if err := s.store.SaveManifest(ctx, sha, manifest); err != nil {
		return entity.Session{}, err
	}
	return sess, nil
}

func (s *Service) Get(ctx context.Context, sha string) (entity.Session, error) {
	return s.store.GetSession(ctx, sha)
}

func (s *Service) List(ctx context.Context, mode string, limit int) ([]entity.Session, error) {
	return s.store.ListSessions(ctx, storage.SessionFilter{Mode: mode, Limit: limit})
}

func (s *Service) Delete(ctx context.Context, sha string) error {
	return s.store.UpdateSessionState(ctx, sha, entity.StateFailed)
}

func newSHA() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
