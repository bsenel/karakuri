package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// Phase 37 step 5, slice 1: POST /objectives/{id}/pause and /resume hand the
// authenticated principal to the service as its own argument. The service's
// stop event is how that is seen from here, because ReconcileHandler holds
// the concrete service. The reason is stored as the caller wrote it, no
// longer suffixed with "(paused by <id>)": who paused is on the record apart
// from why. Nothing in the repository reads that suffix back.

const pauseAuditObjective objective.ObjectiveID = "obj-pause-audit"

// pauseAuditWorld is a reconcile handler over a fresh database with one
// standing objective declared and idle.
type pauseAuditWorld struct {
	store  storage.StorageAdapter
	svc    *featurereconcile.Service
	router chi.Router
}

func newPauseAuditWorld(t *testing.T) *pauseAuditWorld {
	t.Helper()
	store, svc := newStopReconcile(t, newStoppingLoops())
	if err := svc.Declare(context.Background(), objective.Objective{
		ID:      pauseAuditObjective,
		Mode:    objective.ModeStanding,
		Cadence: &objective.Cadence{Every: "1h"},
	}); err != nil {
		t.Fatalf("declare: %v", err)
	}
	h := &handler.ReconcileHandler{Reconcile: svc, Store: store}
	r := chi.NewRouter()
	r.Post("/api/v1/objectives/{id}/pause", h.Pause)
	r.Post("/api/v1/objectives/{id}/resume", h.Resume)
	return &pauseAuditWorld{store: store, svc: svc, router: r}
}

// post sends body to path as principal; an empty principal sends none.
func (w *pauseAuditWorld) post(t *testing.T, principal, path, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if principal != "" {
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: principal}))
	}
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body %s", path, rec.Code, rec.Body.String())
	}
}

// stopEventsFor lists the stop events of control, oldest first, with their
// payloads decoded.
func (w *pauseAuditWorld) stopEventsFor(t *testing.T, control string) []map[string]any {
	t.Helper()
	rows, err := w.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		Kind:        storage.ToolEventStop,
		OldestFirst: true,
	})
	if err != nil {
		t.Fatalf("list stop events: %v", err)
	}
	var out []map[string]any
	for _, r := range rows {
		p := map[string]any{}
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("stop event %q payload %q: %v", r.ID, r.PayloadJSON, err)
		}
		if p["control"] != control {
			continue
		}
		if r.Approver != p["principal"] {
			t.Errorf("stop event %q: approver %q and payload principal %v differ", r.ID, r.Approver, p["principal"])
		}
		out = append(out, p)
	}
	return out
}

func TestPauseHandsTheCallersPrincipalToTheService(t *testing.T) {
	w := newPauseAuditWorld(t)
	w.post(t, "alice", "/api/v1/objectives/"+string(pauseAuditObjective)+"/pause",
		`{"reason":"wrong branch","principal":"mallory"}`)

	evs := w.stopEventsFor(t, "pause")
	if len(evs) != 1 {
		t.Fatalf("pause left %d pause events, want 1: %+v", len(evs), evs)
	}
	if got := evs[0]["principal"]; got != "alice" {
		t.Errorf("pause event principal = %v, want the authenticated alice", got)
	}
	if got := evs[0]["reason"]; got != "wrong branch" {
		t.Errorf("pause event reason = %v, want the caller's %q", got, "wrong branch")
	}
}

func TestPauseNoLongerFoldsThePrincipalIntoTheReason(t *testing.T) {
	w := newPauseAuditWorld(t)
	w.post(t, "alice", "/api/v1/objectives/"+string(pauseAuditObjective)+"/pause", `{"reason":"wrong branch"}`)

	st, err := w.svc.State(context.Background(), pauseAuditObjective)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if !st.Paused {
		t.Fatal("the objective is not paused")
	}
	if st.PausedReason != "wrong branch" {
		t.Errorf("stored reason = %q, want the caller's %q with no principal folded in", st.PausedReason, "wrong branch")
	}
}

func TestResumeHandsTheCallersPrincipalToTheService(t *testing.T) {
	w := newPauseAuditWorld(t)
	path := "/api/v1/objectives/" + string(pauseAuditObjective)
	w.post(t, "alice", path+"/pause", `{"reason":"wrong branch"}`)
	w.post(t, "bob", path+"/resume", ``)

	evs := w.stopEventsFor(t, "resume")
	if len(evs) != 1 {
		t.Fatalf("resume left %d resume events, want 1: %+v", len(evs), evs)
	}
	if got := evs[0]["principal"]; got != "bob" {
		t.Errorf("resume event principal = %v, want the authenticated bob", got)
	}
	if got := evs[0]["objective_id"]; got != string(pauseAuditObjective) {
		t.Errorf("resume event objective_id = %v, want %q", got, pauseAuditObjective)
	}
}
