package vfs

import (
	"crypto/sha256"
	"fmt"
	"time"
)

type BlobMetadata struct {
	SHA         string
	ContentType string
	Size        int64
	ObjectiveID string
	AgentID     string
	Capability  string
	CreatedAt   time.Time
}

func SHA(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}
