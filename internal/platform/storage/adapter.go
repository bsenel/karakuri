package storage

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/git"
)

type BlobMetadata struct {
	MimeType string
	Size     int64
}

type SessionFilter struct {
	Mode   string
	Limit  int
	Offset int
}

type ArtifactFilter struct {
	SessionSHA string
	Name       string
	Status     string
}

type StorageAdapter interface {
	SaveBlob(ctx context.Context, sha string, content []byte, meta BlobMetadata) error
	GetBlob(ctx context.Context, sha string) ([]byte, BlobMetadata, error)
	SaveManifest(ctx context.Context, sessionSHA string, manifest vfs.Manifest) error
	GetManifest(ctx context.Context, sessionSHA string) (vfs.Manifest, error)
	UpdateArtifactStatus(ctx context.Context, sha string, status vfs.ArtifactStatus) error
	ListSessions(ctx context.Context, filter SessionFilter) ([]entity.Session, error)
	QueryArtifacts(ctx context.Context, filter ArtifactFilter) ([]entity.Artifact, error)
	SaveReview(ctx context.Context, review entity.Review) error
	SaveToolEvent(ctx context.Context, event entity.ToolEvent) error
	SaveCheckpoint(ctx context.Context, cp entity.Checkpoint) error
	ResolveCheckpoint(ctx context.Context, id string, decision entity.CheckpointDecision) error
	SaveActionItem(ctx context.Context, item entity.ActionItem) error
	SaveResearchResult(ctx context.Context, result entity.ResearchResult) error
	SaveWorktree(ctx context.Context, wt git.Worktree) error
	GetWorktree(ctx context.Context, taskID string) (git.Worktree, error)
	ListWorktrees(ctx context.Context, sessionSHA string) ([]git.Worktree, error)
	DeleteWorktree(ctx context.Context, taskID string) error
	SaveSession(ctx context.Context, s entity.Session) error
	GetSession(ctx context.Context, sha string) (entity.Session, error)
	UpdateSessionState(ctx context.Context, sha string, state entity.SessionState) error
	SaveArtifact(ctx context.Context, a entity.Artifact) error
	GetArtifact(ctx context.Context, sha string) (entity.Artifact, error)
	ListCheckpoints(ctx context.Context, sessionSHA string) ([]entity.Checkpoint, error)
	GetReviews(ctx context.Context, sessionSHA string) ([]entity.Review, error)
}

func Now() time.Time { return time.Now().UTC() }
