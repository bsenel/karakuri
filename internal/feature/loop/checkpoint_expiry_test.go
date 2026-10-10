package loop

import (
	"context"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
)

// escalate drives the decide step into creating a checkpoint and returns the
// stored checkpoint with the instants bracketing its creation.
func escalate(t *testing.T, ttl time.Duration) (expiresAt *time.Time, before, after time.Time) {
	t.Helper()
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	sc.svc.checkpointTTL = ttl

	before = time.Now()
	_, paused := stepDecide(context.Background(), sc, threeActions(), nil)
	after = time.Now()
	if !paused {
		t.Fatal("decide did not escalate, so no checkpoint was created")
	}
	id := sc.state.result.CheckpointID
	if id == nil || *id == "" {
		t.Fatal("escalation recorded no checkpoint id")
	}
	cp, err := sc.svc.store.GetCheckpoint(context.Background(), *id)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	return cp.ExpiresAt, before, after
}

// Off is the default, and off means today's checkpoint: no expiry at all.
func TestEscalationCarriesNoExpiryWhenTheDurationIsUnset(t *testing.T) {
	got, _, _ := escalate(t, 0)
	if got != nil {
		t.Errorf("ExpiresAt = %v with no duration configured, want nil", got)
	}
}

// With a duration d the checkpoint lapses d after it was created.
func TestEscalationExpiresTheDurationAfterCreation(t *testing.T) {
	const d = 72 * time.Hour
	got, before, after := escalate(t, d)
	if got == nil {
		t.Fatalf("ExpiresAt = nil with the duration set to %s", d)
	}
	// The store keeps less than a nanosecond; allow for it on both sides.
	lo, hi := before.Add(d).Add(-time.Second), after.Add(d).Add(time.Second)
	if got.Before(lo) || got.After(hi) {
		t.Errorf("ExpiresAt = %v, want creation time plus %s (between %v and %v)", got, d, lo, hi)
	}
}
