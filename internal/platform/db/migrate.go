package db

import (
	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"gorm.io/gorm"
)

func RunMigrations(db *gorm.DB, _ string) error {
	return db.AutoMigrate(
		&schema.TwinModel{},
		&schema.ObjectiveModel{},
		&schema.LoopIterationModel{},
		&schema.MemoryEpisodicModel{},
		&schema.MemoryProceduralModel{},
		&schema.MemorySemanticModel{},
		&schema.CheckpointModel{},
		&schema.BlobModel{},
		&schema.WorktreeModel{},
		&schema.ToolEventModel{},
		&schema.LoopStateModel{},
	)
}
