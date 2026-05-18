package vfs

import "context"

type ArtifactStatus string

const (
	StatusDraft    ArtifactStatus = "draft"
	StatusReview   ArtifactStatus = "review"
	StatusApproved ArtifactStatus = "approved"
	StatusRejected ArtifactStatus = "rejected"
	StatusSkipped  ArtifactStatus = "skipped"
)

type Manifest struct {
	SessionSHA string            `json:"session_sha"`
	Mode       string            `json:"mode"`
	ParentSHA  string            `json:"parent_sha,omitempty"`
	Artifacts  map[string]string `json:"artifacts"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

type Service interface {
	Write(ctx context.Context, sessionSHA, name string, content []byte) (sha string, err error)
	Read(ctx context.Context, sha string) ([]byte, error)
	GetManifest(ctx context.Context, sessionSHA string) (Manifest, error)
	LinkArtifact(ctx context.Context, sessionSHA, name, sha string) error
}

func SHA256Content(content []byte) string {
	// delegated to platform storage
	return ""
}
