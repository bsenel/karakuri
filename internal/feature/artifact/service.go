package artifact

import (
	"bytes"
	"context"
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

func (s *Service) Write(ctx context.Context, sessionSHA, name, role string, content []byte) (entity.Artifact, error) {
	sha := storage.ContentSHA([]byte(sessionSHA + ":" + name + ":" + string(content)))
	if err := s.store.SaveBlob(ctx, sha, content, storage.BlobMetadata{MimeType: "text/plain", Size: int64(len(content))}); err != nil {
		return entity.Artifact{}, err
	}
	art := entity.Artifact{
		SHA: sha, SessionSHA: sessionSHA, Name: name, Status: string(vfs.StatusDraft),
		Role: role, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveArtifact(ctx, art); err != nil {
		return entity.Artifact{}, err
	}
	manifest, err := s.store.GetManifest(ctx, sessionSHA)
	if err != nil {
		return entity.Artifact{}, err
	}
	if manifest.Artifacts == nil {
		manifest.Artifacts = make(map[string]string)
	}
	manifest.Artifacts[name] = sha
	if err := s.store.SaveManifest(ctx, sessionSHA, manifest); err != nil {
		return entity.Artifact{}, err
	}
	return art, nil
}

func (s *Service) Read(ctx context.Context, sha string) ([]byte, error) {
	content, _, err := s.store.GetBlob(ctx, sha)
	return content, err
}

func (s *Service) List(ctx context.Context, sessionSHA string) ([]entity.Artifact, error) {
	return s.store.QueryArtifacts(ctx, storage.ArtifactFilter{SessionSHA: sessionSHA})
}

func (s *Service) Diff(ctx context.Context, shaA, shaB string) (string, error) {
	a, err := s.Read(ctx, shaA)
	if err != nil {
		return "", err
	}
	b, err := s.Read(ctx, shaB)
	if err != nil {
		return "", err
	}
	if bytes.Equal(a, b) {
		return "", nil
	}
	return string(a) + "\n---\n" + string(b), nil
}

func (s *Service) Approve(ctx context.Context, sha string) error {
	return s.store.UpdateArtifactStatus(ctx, sha, vfs.StatusApproved)
}
