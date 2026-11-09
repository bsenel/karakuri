package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/go-chi/chi/v5"
)

// Phase 37 step 3, slice B1: POST /loops/{id}/stop with {"reason": ...}.
//
// The reason is handled the way ReconcileHandler.Pause handles it: optional,
// a body that does not decode is read as no reason, and a missing reason is
// filled in from the principal ("paused by <id>" there, "stopped by <id>"
// here). Unlike Pause, the principal is not folded into a given reason,
// because Stop takes it as its own argument and stores it apart.
//
// The answer is the featureloop.StopResult that Stop returned: 200 with
// found "running" when the stop ended the loop, 200 with found "ended" when
// the loop had already ended and nothing changed, 404 when the loop is
// unknown, and 500 when Stop fails.

// stopCall is one call the handler made to Stop.
type stopCall struct {
	loopID, principal, reason string
}

// stoppingLoops is a loop service whose Stop answers from a table and records
// every call; the rest of the service is not reached by the stop route.
type stoppingLoops struct {
	found map[string]featureloop.StopFound
	err   error
	calls []stopCall
}

func (s *stoppingLoops) Stop(_ context.Context, loopID, principal, reason string) (featureloop.StopResult, error) {
	s.calls = append(s.calls, stopCall{loopID, principal, reason})
	res := featureloop.StopResult{LoopID: loopID, Found: featureloop.StopFoundUnknown}
	if f, ok := s.found[loopID]; ok {
		res.Found = f
	}
	return res, s.err
}

func (s *stoppingLoops) Run(context.Context, loop.Request) (loop.Result, error) {
	return loop.Result{}, errors.New("not reached by the stop route")
}

func (s *stoppingLoops) Resume(context.Context, string, corecheckpoint.Decision) (loop.Result, error) {
	return loop.Result{}, errors.New("not reached by the stop route")
}

func (s *stoppingLoops) Status(context.Context, string) (loop.Status, error) {
	return loop.Status{}, errors.New("not reached by the stop route")
}

func (s *stoppingLoops) ResumeCheckpoint(context.Context, string, corecheckpoint.Decision) (bool, error) {
	return false, errors.New("not reached by the stop route")
}

func (s *stoppingLoops) ResumeStoredLoops(context.Context) error {
	return errors.New("not reached by the stop route")
}

var _ featureloop.Service = (*stoppingLoops)(nil)

func newStoppingLoops() *stoppingLoops {
	return &stoppingLoops{found: map[string]featureloop.StopFound{
		"loop-running": featureloop.StopFoundRunning,
		"loop-ended":   featureloop.StopFoundEnded,
	}}
}

// postStop sends body to POST /api/v1/loops/{id}/stop as principal, on the
// path the server mounts. An empty principal sends the request without one.
func postStop(t *testing.T, loops *stoppingLoops, principal, loopID, body string) *httptest.ResponseRecorder {
	t.Helper()
	store, reconcileSvc := newStopReconcile(t, loops)
	h := &handler.LoopHandler{Loop: loops, Store: store, Reconcile: reconcileSvc}
	r := chi.NewRouter()
	r.Post("/api/v1/loops/{id}/stop", h.Stop)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/loops/"+loopID+"/stop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if principal != "" {
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: principal}))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// newStopReconcile is the reconcile service the stop route calls, over a
// fresh database and the given loop service.
func newStopReconcile(t *testing.T, loops *stoppingLoops) (storage.StorageAdapter, *featurereconcile.Service) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "loop_stop.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	hub := event.NewHub()
	rec := featurereconcile.NewService(store, loops, environment.NewRegistry(), nil,
		featurecp.NewService(store, hub), hub, karakuriquota.Deps{}, featurereconcile.Config{})
	return store, rec
}

func decodeStopResult(t *testing.T, rec *httptest.ResponseRecorder) featureloop.StopResult {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json; body %s", ct, rec.Body.String())
	}
	var got featureloop.StopResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return got
}

func onlyCall(t *testing.T, loops *stoppingLoops) stopCall {
	t.Helper()
	if len(loops.calls) != 1 {
		t.Fatalf("Stop called %d times, want once: %+v", len(loops.calls), loops.calls)
	}
	return loops.calls[0]
}

func TestLoopStopEndsARunningLoop(t *testing.T) {
	loops := newStoppingLoops()
	rec := postStop(t, loops, "alice", "loop-running", `{"reason":"started against a merged branch"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	want := stopCall{loopID: "loop-running", principal: "alice", reason: "started against a merged branch"}
	if got := onlyCall(t, loops); got != want {
		t.Fatalf("Stop called with %+v, want %+v", got, want)
	}
	got := decodeStopResult(t, rec)
	if got.LoopID != "loop-running" || got.Found != featureloop.StopFoundRunning {
		t.Fatalf("answer = %+v, want loop-running found running", got)
	}
}

// The principal is the authenticated one, never a field of the body: who
// stopped a run is on the record, so it cannot be self-asserted (the same
// rule Resume follows for its approver, SECURITY_AUDIT.md F-10).
func TestLoopStopTakesThePrincipalFromTheRequestNotTheBody(t *testing.T) {
	loops := newStoppingLoops()
	rec := postStop(t, loops, "alice", "loop-running", `{"reason":"wrong branch","principal":"mallory","stopped_by":"mallory"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if got := onlyCall(t, loops); got.principal != "alice" {
		t.Fatalf("Stop called as %q, want the authenticated alice", got.principal)
	}
}

func TestLoopStopUnknownLoopIs404(t *testing.T) {
	loops := newStoppingLoops()
	rec := postStop(t, loops, "alice", "loop-nobody-knows", `{"reason":"wrong branch"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if got := onlyCall(t, loops); got.loopID != "loop-nobody-knows" {
		t.Fatalf("Stop called for %q", got.loopID)
	}
	if !strings.Contains(rec.Body.String(), "loop-nobody-knows") {
		t.Errorf("404 body %q does not name the loop", rec.Body.String())
	}
}

// Stopping a loop that has already ended is not an error: the stop's promise
// (no further action starts) already holds. The answer says nothing changed.
func TestLoopStopAlreadyEndedLoopSaysSo(t *testing.T) {
	loops := newStoppingLoops()
	rec := postStop(t, loops, "alice", "loop-ended", `{"reason":"wrong branch"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	got := decodeStopResult(t, rec)
	if got.LoopID != "loop-ended" || got.Found != featureloop.StopFoundEnded {
		t.Fatalf("answer = %+v, want loop-ended found ended", got)
	}
}

// A missing reason is not refused, as with pausing an objective: the reason
// is filled in from the principal, so the record still says who and is never
// blank.
func TestLoopStopWithoutAReasonIsFilledInFromThePrincipal(t *testing.T) {
	for name, body := range map[string]string{
		"empty object": `{}`,
		"empty reason": `{"reason":""}`,
		"no body":      ``,
		"not json":     `stop it`,
	} {
		t.Run(name, func(t *testing.T) {
			loops := newStoppingLoops()
			rec := postStop(t, loops, "alice", "loop-running", body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 as Pause gives; body %s", rec.Code, rec.Body.String())
			}
			want := stopCall{loopID: "loop-running", principal: "alice", reason: "stopped by alice"}
			if got := onlyCall(t, loops); got != want {
				t.Fatalf("Stop called with %+v, want %+v", got, want)
			}
		})
	}
}

// A stop that Stop could not complete, or could not make durable, is a 500
// that carries Stop's error, so the caller does not read it as done.
func TestLoopStopFailureIs500(t *testing.T) {
	loops := newStoppingLoops()
	loops.err = errors.New(`loop "loop-running" stopped, but its stopped state was not stored: disk full`)
	rec := postStop(t, loops, "alice", "loop-running", `{"reason":"wrong branch"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "was not stored") {
		t.Errorf("500 body %q does not carry Stop's error", rec.Body.String())
	}
}

// The owner's decision on #222: a stop must stop a watch-mode run too. A loop
// of a standing objective (what POST /loops with watch_mode declares) is
// stopped through the reconcile service, which also pauses its objective, and
// the answer names the objective and says it was paused, so the caller knows
// no later tick starts a new pass.
func TestLoopStopOfAStandingObjectiveNamesItPaused(t *testing.T) {
	loops := newStoppingLoops()
	loops.found["loop-watch"] = featureloop.StopFoundRunning
	store, rec := newStopReconcile(t, loops)

	ctx := context.Background()
	obj := objective.Objective{
		ID:       "obj-watch",
		Mode:     objective.ModeStanding,
		Domain:   "software",
		Cadence:  &objective.Cadence{Sense: "30s"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomySense, Ceiling: objective.AutonomySense},
	}
	if err := store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	if err := rec.Declare(ctx, obj); err != nil {
		t.Fatalf("declare: %v", err)
	}
	if err := store.SaveLoopState(ctx, loop.State{
		LoopID:      "loop-watch",
		ObjectiveID: obj.ID,
		Completed:   true,
		Status:      objective.StatusCompleted,
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}

	h := &handler.LoopHandler{Loop: loops, Store: store, Reconcile: rec}
	r := chi.NewRouter()
	r.Post("/api/v1/loops/{id}/stop", h.Stop)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/loops/loop-watch/stop",
		strings.NewReader(`{"reason":"watching the wrong repository"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: "alice"}))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", resp.Code, resp.Body.String())
	}
	want := stopCall{loopID: "loop-watch", principal: "alice", reason: "watching the wrong repository"}
	if got := onlyCall(t, loops); got != want {
		t.Fatalf("Stop called with %+v, want %+v", got, want)
	}

	var got struct {
		LoopID          string `json:"loop_id"`
		Found           string `json:"found"`
		ObjectiveID     string `json:"objective_id"`
		ObjectivePaused *bool  `json:"objective_paused"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", resp.Body.String(), err)
	}
	if got.LoopID != "loop-watch" || got.Found != string(featureloop.StopFoundRunning) {
		t.Errorf("answer = %s, want loop-watch found running", resp.Body.String())
	}
	if got.ObjectiveID != string(obj.ID) {
		t.Errorf("objective_id = %q, want the loop's standing objective %q; body %s",
			got.ObjectiveID, obj.ID, resp.Body.String())
	}
	if got.ObjectivePaused == nil || !*got.ObjectivePaused {
		t.Errorf("objective_paused missing or false, want true; body %s", resp.Body.String())
	}

	st, err := store.GetReconcileState(ctx, obj.ID)
	if err != nil {
		t.Fatalf("get reconcile state: %v", err)
	}
	if !st.Paused || st.PausedReason != "watching the wrong repository" {
		t.Errorf("objective after the stop: paused %v, reason %q; want paused with the stop's reason",
			st.Paused, st.PausedReason)
	}
}
