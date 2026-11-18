package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 40 step 4's acceptance across a real restart: a server stops while a
// standing objective's loop is in its act step, a second server starts on the
// same database, and that loop is ended with interrupted_by_restart, the
// objective is not deferred, the next tick runs a pass, and
// ConsecutiveFailures is unchanged.
//
// The act step is the row the first process leaves when it dies there: the
// loop not completed, its last step act, and the supervisor still holding it.
// Driving the mock model into an act step and killing it there would test the
// mock, not the restart.
func TestARestartDuringAnActStepEndsTheLoopAndTheNextTickRunsAPass(t *testing.T) {
	const (
		loopID       = "loop-cut-off-by-restart"
		priorFailing = 2
	)
	dbPath := filepath.Join(t.TempDir(), "restart.db")
	withDB := func(cfg *config.Config) { cfg.Database.DSN = dbPath }
	ctx := context.Background()

	// The first process: a standing objective, declared over HTTP.
	baseURL, token, stopFirst := startServerWith(t, withDB)
	twinID := createTwin(t, baseURL, token, "restart-twin")
	objID := createObjective(t, baseURL, token, twinID, "keep the build green")
	resp := doJSON(t, token, http.MethodPut, baseURL+"/api/v1/objectives/"+objID+"/standing", map[string]any{
		"cadence":  map[string]any{"sense": "1h", "min_interval": "30m"},
		"autonomy": map[string]any{"level": "propose", "ceiling": "act_with_notice"},
	})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	gormDB, err := platformdb.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	store := storage.NewGORMStorage(gormDB)
	id := objective.ObjectiveID(objID)
	obj, err := store.GetObjective(ctx, id)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}

	// The pass the first process was running: its loop in act, held by the
	// supervisor, the count one short of a breaker of three.
	req, err := json.Marshal(coreloop.Request{Objective: obj, MaxIter: 5})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := store.SaveLoopState(ctx, coreloop.State{
		LoopID:      loopID,
		ObjectiveID: id,
		Iteration:   1,
		LastStep:    coreloop.StepAct,
		Status:      objective.StatusActive,
		RequestJSON: string(req),
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
	st, err := store.GetReconcileState(ctx, id)
	if err != nil {
		t.Fatalf("get reconcile state: %v", err)
	}
	ranAt := time.Now().UTC().Add(-time.Minute)
	st.ActiveLoopID = loopID
	st.ConsecutiveFailures = priorFailing
	st.Phase = reconcile.PhaseReconciling
	st.LastTrigger = reconcile.TriggerSchedule
	st.LastRunAt, st.LastReconciledAt = &ranAt, &ranAt
	// The re-arm a pass sets while it waits on its loop has passed.
	due := time.Now().UTC().Add(-time.Second)
	st.NextDueAt, st.NextReconcileAt = &due, &due
	// The dead process's lease has run out by the time the next one ticks.
	st.LeaseUntil = &due
	if err := store.SaveReconcileState(ctx, st); err != nil {
		t.Fatalf("save reconcile state: %v", err)
	}

	// The server stops during the act step.
	stopFirst()

	// The second process on the same database, booting as bootstrap does.
	second, _, removeSecond := newTestApp(t, withDB)
	defer removeSecond()
	if err := second.Loop.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}

	ls, err := store.GetLoopState(ctx, loopID)
	if err != nil {
		t.Fatalf("get loop state: %v", err)
	}
	if !ls.Completed || ls.Outcome != coreloop.RunInterruptedByRestart {
		t.Fatalf("after the restart the loop reads completed = %v, outcome = %q; want ended with %q",
			ls.Completed, ls.Outcome, coreloop.RunInterruptedByRestart)
	}
	if ls.Error != "" || ls.Status == objective.StatusFailed {
		t.Errorf("loop error = %q, status = %q; a restart is not a failure", ls.Error, ls.Status)
	}

	// The next tick runs a fresh pass for the stream.
	second.Reconcile.Tick(ctx)
	var after reconcile.State
	deadline := time.Now().Add(30 * time.Second)
	for {
		after, err = store.GetReconcileState(ctx, id)
		if err != nil {
			t.Fatalf("get reconcile state: %v", err)
		}
		if after.ActiveLoopID != loopID && after.LastRunAt != nil && after.LastRunAt.After(ranAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no pass ran on the tick after the restart: active loop %q, last run %v, phase %q",
				after.ActiveLoopID, after.LastRunAt, after.Phase)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if after.ConsecutiveFailures != priorFailing {
		t.Errorf("consecutive_failures = %d after the restart, want %d unchanged", after.ConsecutiveFailures, priorFailing)
	}
	if after.Paused {
		t.Errorf("the stream was paused (%s); a restart is not a failure", after.PausedReason)
	}
	history, err := second.Reconcile.History(ctx, id, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, o := range history {
		if o.Deferred != "" {
			t.Errorf("a pass was recorded as deferred (%q); the stream whose loop the restart ended is run again", o.Deferred)
		}
	}
}
