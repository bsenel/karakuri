// Package migrate copies all Karakuri persistence between two database backends
// (e.g. SQLite → PostgreSQL). Migration runs at the GORM model layer so every
// column is preserved exactly; the higher-level StorageAdapter DTOs would lose
// fields that aren't part of the public interface.
package migrate

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/db/schema"
)

// Plan describes a migration job (source and destination DSNs).
type Plan struct {
	FromDriver string
	FromDSN    string
	ToDriver   string
	ToDSN      string
	BatchSize  int // defaults to 200
}

// Report summarizes a completed migration.
type Report struct {
	Tables map[string]int `json:"tables"`
}

// Run executes the migration. The destination database receives a fresh
// schema (AutoMigrate) before any rows are copied.
func Run(ctx context.Context, p Plan) (Report, error) {
	if p.BatchSize <= 0 {
		p.BatchSize = 200
	}

	src, err := db.Open(p.FromDriver, p.FromDSN)
	if err != nil {
		return Report{}, fmt.Errorf("open source (%s): %w", p.FromDriver, err)
	}
	dst, err := db.Open(p.ToDriver, p.ToDSN)
	if err != nil {
		return Report{}, fmt.Errorf("open target (%s): %w", p.ToDriver, err)
	}
	if err := db.RunMigrations(dst, p.ToDSN); err != nil {
		return Report{}, fmt.Errorf("create target schema: %w", err)
	}

	report := Report{Tables: map[string]int{}}

	if err := copyTable[schema.TwinModel](ctx, src, dst, p.BatchSize, "twins", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.ObjectiveModel](ctx, src, dst, p.BatchSize, "objectives", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.LoopIterationModel](ctx, src, dst, p.BatchSize, "loop_iterations", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.MemoryEpisodicModel](ctx, src, dst, p.BatchSize, "memory_episodic", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.MemoryProceduralModel](ctx, src, dst, p.BatchSize, "memory_procedural", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.MemorySemanticModel](ctx, src, dst, p.BatchSize, "memory_semantic", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.CheckpointModel](ctx, src, dst, p.BatchSize, "checkpoints", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.BlobModel](ctx, src, dst, p.BatchSize, "blobs", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.WorktreeModel](ctx, src, dst, p.BatchSize, "worktrees", report.Tables); err != nil {
		return report, err
	}
	if err := copyTable[schema.ToolEventModel](ctx, src, dst, p.BatchSize, "tool_events", report.Tables); err != nil {
		return report, err
	}

	return report, nil
}

// copyTable streams every row of a GORM model from src to dst in batches.
// Generic over the model type so each table's specific column set is preserved
// without manual conversion code per table.
func copyTable[T any](ctx context.Context, src, dst *gorm.DB, batch int, label string, counts map[string]int) error {
	var rows []T
	if err := src.WithContext(ctx).FindInBatches(&rows, batch, func(_ *gorm.DB, _ int) error {
		if len(rows) == 0 {
			return nil
		}
		// CreateInBatches uses INSERT; dst was AutoMigrate'd just before so the
		// table is empty. If callers ever support incremental migration we'd
		// need to switch to UPSERT (clause.OnConflict).
		if err := dst.WithContext(ctx).CreateInBatches(rows, batch).Error; err != nil {
			return fmt.Errorf("%s: insert batch: %w", label, err)
		}
		counts[label] += len(rows)
		return nil
	}).Error; err != nil {
		return fmt.Errorf("%s: scan source: %w", label, err)
	}
	return nil
}
