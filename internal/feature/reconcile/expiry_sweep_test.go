package reconcile

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// fakeSweeper records what the tick asked it, and answers with err.
type fakeSweeper struct {
	mu    sync.Mutex
	calls []time.Time
	err   error
}

func (f *fakeSweeper) ExpireDue(_ context.Context, now time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, now)
	return 0, f.err
}

func (f *fakeSweeper) seen() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.calls...)
}

// dueObjective declares a standing objective that the next tick will dispatch.
func dueObjective(t *testing.T, f *fixture) objective.Objective {
	t.Helper()
	f.use(t, map[string]string{"git": "aaa"})
	return f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
}

// waitForLoops waits for the tick's background dispatch to reach the loop.
func waitForLoops(t *testing.T, f *fixture, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.loops.count() < want {
		if time.Now().After(deadline) {
			t.Fatalf("tick dispatched %d loops, want %d", f.loops.count(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// The sweep rides the tick the supervisor already has, and is told the
// supervisor's clock rather than the wall's.
func TestTickCallsTheSweeperWithTheClocksNow(t *testing.T) {
	f := newFixture(t, Config{})
	sw := &fakeSweeper{}
	f.svc.WithCheckpointSweeper(sw)

	f.clock = f.clock.Add(31 * time.Second)
	f.svc.Tick(context.Background())

	calls := sw.seen()
	if len(calls) != 1 {
		t.Fatalf("sweeper called %d times in one tick, want 1", len(calls))
	}
	if !calls[0].Equal(f.clock) {
		t.Errorf("sweeper was told now = %v, want the supervisor's clock %v", calls[0], f.clock)
	}

	f.clock = f.clock.Add(31 * time.Second)
	f.svc.Tick(context.Background())
	calls = sw.seen()
	if len(calls) != 2 || !calls[1].Equal(f.clock) {
		t.Errorf("second tick: sweeper calls = %v, want a second one at %v", calls, f.clock)
	}
}

// Expiry off is today's supervisor: the tick still dispatches what is due.
func TestTickWithoutASweeperBehavesAsBefore(t *testing.T) {
	f := newFixture(t, Config{})
	dueObjective(t, f)

	f.svc.Tick(context.Background())

	waitForLoops(t, f, 1)
}

// A sweep that fails is a warning, not a reason to stop holding objectives.
func TestSweeperErrorDoesNotStopTheTick(t *testing.T) {
	f := newFixture(t, Config{})
	sw := &fakeSweeper{err: errors.New("store is down")}
	f.svc.WithCheckpointSweeper(sw)
	dueObjective(t, f)

	f.svc.Tick(context.Background())

	if got := len(sw.seen()); got != 1 {
		t.Fatalf("sweeper called %d times, want 1", got)
	}
	waitForLoops(t, f, 1)
}

// pendingCheckpoint stores a pending checkpoint through the real service.
func pendingCheckpoint(t *testing.T, f *fixture, cps *featurecp.Service, expiresAt *time.Time) corecheckpoint.Checkpoint {
	t.Helper()
	cp, err := cps.Create(context.Background(), "obj-expiry", "twin-expiry",
		"authority_exceeded", "wants to force-push",
		[]string{"approve", "reject"}, featurecp.CreateOptions{ExpiresAt: expiresAt})
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	return cp
}

func rejectionRows(t *testing.T, f *fixture) []storage.ToolEvent {
	t.Helper()
	rows, err := f.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: "obj-expiry",
	})
	if err != nil {
		t.Fatalf("list audit rows: %v", err)
	}
	var out []storage.ToolEvent
	for _, r := range rows {
		if r.Kind == storage.ToolEventRejection {
			out = append(out, r)
		}
	}
	return out
}

// The whole of step 2 over the real checkpoint service and the sqlite store:
// one tick ends a checkpoint nobody answered, as a rejection, on the record.
func TestTickExpiresAnUnansweredCheckpoint(t *testing.T) {
	f := newFixture(t, Config{})
	cps := featurecp.NewService(f.store, event.NewHub())
	f.svc.WithCheckpointSweeper(cps)

	due := f.clock.Add(time.Hour)
	cp := pendingCheckpoint(t, f, cps, &due)

	// Before its time the tick leaves it alone.
	f.svc.Tick(context.Background())
	if got, err := f.store.GetCheckpoint(context.Background(), cp.ID); err != nil {
		t.Fatalf("get checkpoint: %v", err)
	} else if got.Status != corecheckpoint.StatusPending {
		t.Fatalf("checkpoint is %q before its time, want pending", got.Status)
	}

	f.clock = due.Add(time.Second)
	f.svc.Tick(context.Background())

	got, err := f.store.GetCheckpoint(context.Background(), cp.ID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if got.Status != corecheckpoint.StatusResolved {
		t.Fatalf("checkpoint is %q one tick past its time, want resolved", got.Status)
	}
	if got.Decision == nil || got.Decision.Choice != "reject" || got.Decision.Approver != "system:timeout" {
		t.Errorf("decision = %+v, want reject by system:timeout", got.Decision)
	}
	rows := rejectionRows(t, f)
	if len(rows) != 1 {
		t.Fatalf("%d rejection audit rows, want 1", len(rows))
	}
	if rows[0].Approver != "system:timeout" {
		t.Errorf("audit approver = %q, want system:timeout", rows[0].Approver)
	}
}

// With the duration unset nothing is wired and nothing carries an expiry, so
// the same tick, however late, expires nothing.
func TestTickExpiresNothingWhenExpiryIsOff(t *testing.T) {
	f := newFixture(t, Config{})
	cps := featurecp.NewService(f.store, event.NewHub())
	cp := pendingCheckpoint(t, f, cps, nil)

	f.clock = f.clock.Add(365 * 24 * time.Hour)
	f.svc.Tick(context.Background())

	got, err := f.store.GetCheckpoint(context.Background(), cp.ID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if got.Status != corecheckpoint.StatusPending {
		t.Errorf("checkpoint is %q with expiry off, want pending", got.Status)
	}
	if rows := rejectionRows(t, f); len(rows) != 0 {
		t.Errorf("%d rejection audit rows with expiry off, want 0", len(rows))
	}
}
