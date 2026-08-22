package storage_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// The lease is the whole of Karakuri's distributed coordination, and it is one
// SQL statement. It is therefore exercised against a real database rather than
// asserted about: a conditional UPDATE that silently matches two rows is not a
// bug any mock would reproduce.
func newReconcileStore(t *testing.T) *storage.GORMStorage {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "reconcile.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.NewGORMStorage(db)
}

func seedState(t *testing.T, s *storage.GORMStorage, id string, dueAt *time.Time) {
	t.Helper()
	err := s.SaveReconcileState(context.Background(), reconcile.State{
		ObjectiveID: coreobjective.ObjectiveID(id),
		TwinID:      "twin-1",
		Phase:       reconcile.PhaseIdle,
		NextDueAt:   dueAt,
	})
	if err != nil {
		t.Fatalf("seed %q: %v", id, err)
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestReconcileStateRoundTrip(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	want := reconcile.State{
		ObjectiveID:  "obj-1",
		TwinID:       "twin-1",
		Phase:        reconcile.PhaseWaiting,
		Paused:       true,
		PausedReason: "circuit breaker: 3 consecutive failures",
		NextDueAt:    ptr(now.Add(time.Hour)),
		NextSenseAt:  ptr(now.Add(15 * time.Minute)),
		Converged: reconcile.Fingerprint{
			SHA:          "abc123",
			Environments: map[string]string{"git": "deadbeef", "ticket": "cafe"},
			Blind:        []string{"calendar"},
			TakenAt:      now,
		},
		LastConvergedAt:     ptr(now.Add(-time.Hour)),
		LastTrigger:         reconcile.TriggerDrift,
		LastError:           "adapter timeout",
		CriteriaMet:         0.75,
		ScoreStreak:         2,
		ConsecutiveFailures: 3,
		Autonomy:            coreobjective.AutonomyActWithNotice,
		CleanRuns:           4,
	}
	if err := s.SaveReconcileState(ctx, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetReconcileState(ctx, "obj-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Phase != want.Phase || got.Paused != want.Paused || got.PausedReason != want.PausedReason {
		t.Errorf("phase/pause = %v/%v/%q, want %v/%v/%q",
			got.Phase, got.Paused, got.PausedReason, want.Phase, want.Paused, want.PausedReason)
	}
	if got.Converged.SHA != "abc123" || got.Converged.Environments["git"] != "deadbeef" {
		t.Errorf("converged fingerprint = %+v, want the one saved", got.Converged)
	}
	if len(got.Converged.Blind) != 1 || got.Converged.Blind[0] != "calendar" {
		t.Errorf("blind environments = %v, want [calendar]", got.Converged.Blind)
	}
	if got.CriteriaMet != 0.75 || got.ScoreStreak != 2 || got.ConsecutiveFailures != 3 {
		t.Errorf("counters = %v/%d/%d, want 0.75/2/3", got.CriteriaMet, got.ScoreStreak, got.ConsecutiveFailures)
	}
	if got.Autonomy != coreobjective.AutonomyActWithNotice || got.CleanRuns != 4 {
		t.Errorf("autonomy = %q clean_runs = %d, want act_with_notice/4", got.Autonomy, got.CleanRuns)
	}
	if got.NextDueAt == nil || !got.NextDueAt.Equal(*want.NextDueAt) {
		t.Errorf("next_due_at = %v, want %v", got.NextDueAt, want.NextDueAt)
	}
	if got.NextReconcileAt != nil {
		t.Errorf("next_reconcile_at = %v, want nil — an unset time must not become the epoch", got.NextReconcileAt)
	}
}

// A standing objective that declared no schedule is never due on its own. It
// must not read as overdue since year one.
func TestNullNextDueIsNeverListed(t *testing.T) {
	s := newReconcileStore(t)
	now := time.Now().UTC()

	seedState(t, s, "unscheduled", nil)
	seedState(t, s, "due", ptr(now.Add(-time.Minute)))

	due, err := s.ListDueReconcileStates(context.Background(), "server-a", now, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(due) != 1 || due[0].ObjectiveID != "due" {
		t.Fatalf("due = %v, want only the scheduled objective", ids(due))
	}
}

func TestListDueExcludesFutureAndPaused(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedState(t, s, "overdue", ptr(now.Add(-time.Hour)))
	seedState(t, s, "future", ptr(now.Add(time.Hour)))

	// Paused, and due: the pause is what must keep it out.
	if err := s.SaveReconcileState(ctx, reconcile.State{
		ObjectiveID: "paused",
		NextDueAt:   ptr(now.Add(-time.Hour)),
		Paused:      true,
	}); err != nil {
		t.Fatalf("seed paused: %v", err)
	}

	due, err := s.ListDueReconcileStates(ctx, "server-a", now, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(due) != 1 || due[0].ObjectiveID != "overdue" {
		t.Fatalf("due = %v, want only [overdue]", ids(due))
	}
}

// When the concurrency limit bites, the most overdue objectives are the ones
// that run. Primary-key order would let one objective starve indefinitely
// while alphabetically earlier neighbours were serviced every tick.
func TestListDueReturnsMostOverdueFirst(t *testing.T) {
	s := newReconcileStore(t)
	now := time.Now().UTC()

	seedState(t, s, "aaa-recent", ptr(now.Add(-time.Minute)))
	seedState(t, s, "zzz-ancient", ptr(now.Add(-24*time.Hour)))

	due, err := s.ListDueReconcileStates(context.Background(), "server-a", now, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(due) != 1 || due[0].ObjectiveID != "zzz-ancient" {
		t.Fatalf("due = %v, want the most overdue objective first", ids(due))
	}
}

// The property the whole design rests on: two replicas racing for one
// objective, and exactly one wins. A second winner means two loops, two bills
// and two copies of the same morning report.
func TestOnlyOneReplicaClaimsAnObjective(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedState(t, s, "contested", ptr(now.Add(-time.Minute)))

	const replicas = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []string
	)
	start := make(chan struct{})
	for i := 0; i < replicas; i++ {
		holder := string(rune('a' + i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, err := s.ClaimReconcileState(ctx, "contested", holder, now, now.Add(time.Minute))
			if err != nil {
				return // SQLite serialises writers; a busy loser is still a loser.
			}
			if ok {
				mu.Lock()
				winners = append(winners, holder)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(winners) != 1 {
		t.Fatalf("%d replicas claimed the objective (%v), want exactly 1", len(winners), winners)
	}

	got, err := s.GetReconcileState(ctx, "contested")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Holder != winners[0] {
		t.Errorf("holder = %q, want the winner %q", got.Holder, winners[0])
	}
}

func TestLeaseLifecycle(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedState(t, s, "obj", ptr(now.Add(-time.Minute)))

	ok, err := s.ClaimReconcileState(ctx, "obj", "server-a", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}

	// A live lease locks everybody else out.
	if ok, err := s.ClaimReconcileState(ctx, "obj", "server-b", now, now.Add(time.Minute)); err != nil || ok {
		t.Errorf("server-b claimed a live lease: ok=%v err=%v", ok, err)
	}

	// The holder may re-claim, which is what makes resume-after-restart work
	// without a special case.
	if ok, err := s.ClaimReconcileState(ctx, "obj", "server-a", now, now.Add(2*time.Minute)); err != nil || !ok {
		t.Errorf("holder could not re-claim: ok=%v err=%v", ok, err)
	}

	// Renewal requires still being the holder.
	if ok, err := s.RenewReconcileLease(ctx, "obj", "server-b", now, now.Add(time.Minute)); err != nil || ok {
		t.Errorf("server-b renewed a lease it does not hold: ok=%v err=%v", ok, err)
	}
	if ok, err := s.RenewReconcileLease(ctx, "obj", "server-a", now, now.Add(5*time.Minute)); err != nil || !ok {
		t.Errorf("holder could not renew: ok=%v err=%v", ok, err)
	}

	// A crashed holder releases nothing; the lease simply runs out.
	later := now.Add(10 * time.Minute)
	if ok, err := s.ClaimReconcileState(ctx, "obj", "server-b", later, later.Add(time.Minute)); err != nil || !ok {
		t.Errorf("server-b could not take over an expired lease: ok=%v err=%v", ok, err)
	}

	// A late release from the displaced replica must not unlock work that
	// server-b is now doing.
	if err := s.ReleaseReconcileLease(ctx, "obj", "server-a"); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, err := s.GetReconcileState(ctx, "obj")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Holder != "server-b" {
		t.Errorf("holder = %q after a stale release, want server-b", got.Holder)
	}

	if err := s.ReleaseReconcileLease(ctx, "obj", "server-b"); err != nil {
		t.Fatalf("release by holder: %v", err)
	}
	if got, _ := s.GetReconcileState(ctx, "obj"); got.Holder != "" || got.LeaseUntil != nil {
		t.Errorf("lease = %q/%v after release, want cleared", got.Holder, got.LeaseUntil)
	}
}

// A replica's own unfinished claim is work it should pick up again, so its own
// leases do not hide rows from it.
func TestListDueIncludesTheCallersOwnLease(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedState(t, s, "mine", ptr(now.Add(-time.Minute)))
	seedState(t, s, "theirs", ptr(now.Add(-time.Minute)))

	if _, err := s.ClaimReconcileState(ctx, "mine", "server-a", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("claim mine: %v", err)
	}
	if _, err := s.ClaimReconcileState(ctx, "theirs", "server-b", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("claim theirs: %v", err)
	}

	due, err := s.ListDueReconcileStates(ctx, "server-a", now, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(due) != 1 || due[0].ObjectiveID != "mine" {
		t.Fatalf("due = %v, want only the caller's own claim", ids(due))
	}
}

func TestReconcileOutcomesAreNewestFirst(t *testing.T) {
	s := newReconcileStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i, at := range []time.Time{now.Add(-3 * time.Hour), now.Add(-time.Hour), now.Add(-2 * time.Hour)} {
		err := s.SaveReconcileOutcome(ctx, reconcile.Outcome{
			ID:          string(rune('a' + i)),
			ObjectiveID: "obj",
			Trigger:     reconcile.TriggerDrift,
			Drift:       reconcile.Drift{Changed: true, Environments: []string{"git"}},
			StartedAt:   at,
			EndedAt:     at.Add(time.Minute),
		})
		if err != nil {
			t.Fatalf("save outcome: %v", err)
		}
	}
	// A sense-only pass on another objective must not appear.
	if err := s.SaveReconcileOutcome(ctx, reconcile.Outcome{
		ID: "other", ObjectiveID: "elsewhere", Trigger: reconcile.TriggerSchedule, StartedAt: now,
	}); err != nil {
		t.Fatalf("save other: %v", err)
	}

	got, err := s.ListReconcileOutcomes(ctx, "obj", 10)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d outcomes, want 3", len(got))
	}
	if got[0].ID != "b" || got[2].ID != "a" {
		t.Errorf("order = %s/%s/%s, want b/c/a (newest first)", got[0].ID, got[1].ID, got[2].ID)
	}
	if !got[0].Drift.Changed || len(got[0].Drift.Environments) != 1 {
		t.Errorf("drift did not round-trip: %+v", got[0].Drift)
	}
}

func ids(states []reconcile.State) []string {
	out := make([]string, len(states))
	for i, s := range states {
		out[i] = string(s.ObjectiveID)
	}
	return out
}
