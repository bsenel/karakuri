package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type LoopHandler struct {
	Loop      featureloop.Service
	Store     storage.StorageAdapter
	Reconcile *featurereconcile.Service
}

func (h *LoopHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ObjectiveID string `json:"objective_id"`
		TwinID      string `json:"twin_id"`
		MaxIter     int    `json:"max_iter"`
		// WatchMode keeps watching after the loop finishes. Phase 20 keeps
		// the field and reimplements it: it now declares the objective
		// standing at sense-only autonomy rather than starting an in-process
		// ticker. Same behaviour — poll the environments, raise a checkpoint
		// when one moves — and it survives a restart, which the ticker,
		// living in the goroutine of a finished loop, did not.
		WatchMode bool `json:"watch_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.WatchMode {
		if err := h.declareWatch(r, objective.ObjectiveID(req.ObjectiveID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result, err := h.Loop.Run(r.Context(), loop.Request{
		Objective: objective.Objective{ID: objective.ObjectiveID(req.ObjectiveID)},
		Twin:      twin.DigitalTwin{ID: req.TwinID},
		MaxIter:   req.MaxIter,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// declareWatch turns the objective into a standing one that only ever watches.
//
// Sense at both level and ceiling is what makes it observe-only for good: the
// ceiling is enforced on every read of the state, so no amount of clean
// history can promote a watcher into something that acts. An operator who
// wants it to act says so by declaring the objective standing themselves, with
// the autonomy they meant.
func (h *LoopHandler) declareWatch(r *http.Request, id objective.ObjectiveID) error {
	if h.Reconcile == nil || h.Store == nil || id == "" {
		return nil
	}
	obj, err := h.Store.GetObjective(r.Context(), id)
	if err != nil {
		return err
	}
	obj.Mode = objective.ModeStanding
	if obj.Cadence == nil {
		// Thirty seconds is what watch mode's ticker used. It is affordable
		// because sensing is adapter calls rather than model calls.
		obj.Cadence = &objective.Cadence{Sense: "30s"}
	}
	if obj.Autonomy == nil {
		obj.Autonomy = &objective.Autonomy{
			Level:   objective.AutonomySense,
			Ceiling: objective.AutonomySense,
		}
	}
	if err := h.Store.SaveObjective(r.Context(), obj); err != nil {
		return err
	}
	return h.Reconcile.Declare(r.Context(), obj)
}

func (h *LoopHandler) Status(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := h.Loop.Status(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, status)
}

func (h *LoopHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Decision      string                        `json:"decision"`
		Note          string                        `json:"note"`
		Approver      string                        `json:"approver"`
		Modifications *corecheckpoint.Modifications `json:"modifications,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The approver is who the audit trail says authorized this resume, so it
	// comes from the authenticated principal, not a self-asserted body field.
	// See SECURITY_AUDIT.md F-10.
	approver := req.Approver
	if p, ok := auth.PrincipalFromContext(r.Context()); ok {
		approver = p.ID
	}
	result, err := h.Loop.Resume(r.Context(), id, corecheckpoint.Decision{
		Choice:        req.Decision,
		Note:          req.Note,
		Approver:      approver,
		Modifications: req.Modifications,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
