package db

import (
	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&schema.SessionModel{},
		&schema.BlobModel{},
		&schema.ManifestModel{},
		&schema.ArtifactModel{},
		&schema.ReviewModel{},
		&schema.ToolEventModel{},
		&schema.CheckpointModel{},
		&schema.ActionItemModel{},
		&schema.ResearchResultModel{},
		&schema.WorktreeModel{},
	)
}
