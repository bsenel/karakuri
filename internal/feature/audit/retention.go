// Package audit owns the retention rule on the audit log (tool_events).
//
// It imports only the standard library so the config package can validate a
// retention setting against the same floor the pruner enforces.
package audit

import (
	"context"
	"time"
)

// FloorDays is the declared retention floor on the audit log: six months,
// because the roadmap's Phase 31 cites a six-month minimum for automatic event
// logs (EU AI Act Article 12). It is configurable upward only.
const FloorDays = 183

// Retention is how long the audit log is kept.
type Retention struct {
	// FloorDays is the floor in force; it may be raised above the package
	// FloorDays, never lowered below it.
	FloorDays int
	// Days is how long audit rows are kept before pruning. 0 means never
	// prune.
	Days int
}

// CheckRetention reports whether r respects the retention floor.
func CheckRetention(r Retention) error {
	return nil
}

// eventPruner is the one storage operation the service needs.
type eventPruner interface {
	DeleteToolEventsBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// Service prunes the audit log, and is the only place that may.
type Service struct {
	store     eventPruner
	retention Retention
}

func NewService(store eventPruner, r Retention) *Service {
	return &Service{store: store, retention: r}
}

// Prune deletes audit rows older than the retention window measured back from
// now and returns how many it removed.
func (s *Service) Prune(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
