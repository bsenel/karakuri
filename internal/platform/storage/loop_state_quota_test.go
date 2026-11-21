package storage_test

import (
	"context"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// A run that ended waiting for quota keeps its outcome and how long the
// provider asked it to wait across a save and a load: reconcile reads the time
// the quota returns from the stored row, never from memory (Phase 40 step 8).
func TestLoopStateKeepsWaitingForQuotaAndRetryAfter(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()

	cases := []struct {
		loopID     string
		retryAfter time.Duration
	}{
		{"loop-quota-said", 2*time.Hour + 30*time.Second},
		{"loop-quota-unsaid", 0},
	}
	for _, tc := range cases {
		err := s.SaveLoopState(ctx, coreloop.State{
			LoopID:      tc.loopID,
			ObjectiveID: objective.ObjectiveID("obj-quota"),
			Completed:   true,
			Status:      objective.StatusFailed,
			Outcome:     coreloop.RunWaitingForQuota,
			RetryAfter:  tc.retryAfter,
		})
		if err != nil {
			t.Fatalf("save %s: %v", tc.loopID, err)
		}
		got, err := s.GetLoopState(ctx, tc.loopID)
		if err != nil {
			t.Fatalf("get %s: %v", tc.loopID, err)
		}
		if got.Outcome != coreloop.RunWaitingForQuota {
			t.Errorf("%s: outcome = %q, want %q", tc.loopID, got.Outcome, coreloop.RunWaitingForQuota)
		}
		if got.RetryAfter != tc.retryAfter {
			t.Errorf("%s: retry_after = %s, want %s", tc.loopID, got.RetryAfter, tc.retryAfter)
		}
	}
}
