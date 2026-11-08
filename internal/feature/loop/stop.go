package loop

import (
	"context"
	"errors"
)

// StopFound is what a stop found the loop to be when it arrived.
type StopFound string

const (
	// StopFoundRunning is a loop that had not ended, including one waiting
	// at a checkpoint; the stop ended it.
	StopFoundRunning StopFound = "running"
	// StopFoundEnded is a loop that had already ended; the stop changed
	// nothing.
	StopFoundEnded StopFound = "ended"
	// StopFoundUnknown is a loop ID no process and no stored row knows.
	StopFoundUnknown StopFound = "unknown"
)

// StopResult is what Stop found and did (Phase 37 step 3).
type StopResult struct {
	LoopID string    `json:"loop_id"`
	Found  StopFound `json:"found"`
}

// Stop ends a one-shot loop on behalf of principal, for reason. Phase 37
// step 3, slice A: the tests in stop_test.go specify it; it is not
// implemented yet.
func (s *serviceImpl) Stop(ctx context.Context, loopID, principal, reason string) (StopResult, error) {
	return StopResult{}, errors.New("not implemented")
}
