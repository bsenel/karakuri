package artifact

import (
	"bytes"
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Artifact struct {
	SHA         string    `json:"sha"`
	ObjectiveID string    `json:"objective_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	Capability  string    `json:"capability,omitempty"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	store storage.StorageAdapter
}

func NewService(store storage.StorageAdapter) *Service {
	return &Service{store: store}
}

func (s *Service) Write(ctx context.Context, objectiveID, agentID, capability string, content []byte) (Artifact, error) {
	sha := vfs.SHA(content)
	meta := vfs.BlobMetadata{
		SHA: sha, ContentType: "text/plain", Size: int64(len(content)),
		ObjectiveID: objectiveID, AgentID: agentID, Capability: capability,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveBlob(ctx, sha, content, meta); err != nil {
		return Artifact{}, err
	}
	return Artifact{
		SHA: sha, ObjectiveID: objectiveID, AgentID: agentID, Capability: capability,
		ContentType: "text/plain", Size: int64(len(content)), CreatedAt: meta.CreatedAt,
	}, nil
}

func (s *Service) Read(ctx context.Context, sha string) ([]byte, vfs.BlobMetadata, error) {
	return s.store.GetBlob(ctx, sha)
}

func (s *Service) List(ctx context.Context, objectiveID, agentID string) ([]Artifact, error) {
	metas, err := s.store.ListBlobs(ctx, objectiveID, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, len(metas))
	for i, m := range metas {
		out[i] = Artifact{
			SHA: m.SHA, ObjectiveID: m.ObjectiveID, AgentID: m.AgentID,
			Capability: m.Capability, ContentType: m.ContentType,
			Size: m.Size, CreatedAt: m.CreatedAt,
		}
	}
	return out, nil
}

func (s *Service) Diff(ctx context.Context, shaA, shaB string) (string, error) {
	a, _, err := s.Read(ctx, shaA)
	if err != nil {
		return "", err
	}
	b, _, err := s.Read(ctx, shaB)
	if err != nil {
		return "", err
	}
	if bytes.Equal(a, b) {
		return "", nil
	}
	return string(a) + "\n---\n" + string(b), nil
}
