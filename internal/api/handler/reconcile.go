package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	"github.com/bsenel/karakuri/internal/platform/schedule"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// ReconcileHandler is the standing-objective surface: declare one, look at
// what its control loop has been doing, ask for a reconcile now, stop it,
// start it again.
type ReconcileHandler struct {
	Reconcile *featurereconcile.Service
	Store     storage.StorageAdapter
}

// Declare makes an objective standing, or edits the declaration of one that
// already is.
//
// PUT rather than POST because it is idempotent and total: the body is the
// whole declaration, and sending it twice leaves the same objective. An
// operator adjusting a cadence sends the cadence they now want, not a patch
// against one they would have to have read first.
func (h *ReconcileHandler) Declare(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	var req struct {
		Cadence  *objective.Cadence  `json:"cadence"`
		Autonomy *objective.Autonomy `json:"autonomy"`
		Budget   *objective.Budget   `json:"budget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	obj, err := h.Store.GetObjective(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate before writing anything. A cadence that will not parse is a
	// standing objective that silently never runs, and that failure is
	// invisible precisely because nothing happening is what it looks like when
	// everything is fine.
	if req.Cadence != nil {
		if err := schedule.Validate(*req.Cadence); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.Autonomy != nil {
		if req.Autonomy.Level != "" && !req.Autonomy.Level.Valid() {
			http.Error(w, "unknown autonomy level "+string(req.Autonomy.Level), http.StatusBadRequest)
			return
		}
		if req.Autonomy.Ceiling != "" && !req.Autonomy.Ceiling.Valid() {
			http.Error(w, "unknown autonomy ceiling "+string(req.Autonomy.Ceiling), http.StatusBadRequest)
			return
		}
	}

	// A negative ceiling is not a smaller budget, it is a typo. Accepting it
	// would read as "no ceiling" everywhere downstream, because every check is
	// `> 0` — an operator meaning to tighten a limit would remove it.
	if req.Budget != nil {
		if req.Budget.Daily < 0 || req.Budget.PerReconcile < 0 {
			http.Error(w, "budget ceilings must not be negative", http.StatusBadRequest)
			return
		}
	}

	obj.Mode = objective.ModeStanding
	obj.Cadence = req.Cadence
	obj.Autonomy = req.Autonomy
	// Total like the rest of the declaration: sending no budget removes the
	// one that was there, because the body is the whole declaration and an
	// operator clearing a ceiling should not need a separate call.
	obj.Budget = req.Budget
	if err := h.Store.SaveObjective(r.Context(), obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Reconcile.Declare(r.Context(), obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	st, err := h.Reconcile.State(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}

// Undeclare returns a standing objective to a one-shot one and drops its
// control loop. The objective and its history survive; only the supervision
// stops.
func (h *ReconcileHandler) Undeclare(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	obj, err := h.Store.GetObjective(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	obj.Mode = objective.ModeOneshot
	obj.Cadence = nil
	obj.Autonomy = nil
	if err := h.Store.SaveObjective(r.Context(), obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Reconcile.Forget(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Get returns the control-loop state and recent history together.
//
// Together rather than as two endpoints because they answer one question — "is
// this being looked after, and what has it been doing" — and an operator who
// has to make two calls to answer it will make one and guess at the other.
func (h *ReconcileHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	st, err := h.Reconcile.State(r.Context(), id)
	if err != nil {
		http.Error(w, "objective is not standing", http.StatusNotFound)
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	history, err := h.Reconcile.History(r.Context(), id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		State   reconcile.State     `json:"state"`
		History []reconcile.Outcome `json:"history"`
	}{State: st, History: history})
}

// Reconcile asks for a pass now, outside the cadence.
//
// It returns 202: the pass runs in the background under the same lease and the
// same concurrency bound as a scheduled one. "Now" means as soon as this
// replica has a slot and nobody else holds the objective, not "in addition to
// whatever is already running".
func (h *ReconcileHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	if err := h.Reconcile.Trigger(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"objective_id": string(id), "status": "reconcile requested"})
}

// Pause stops a standing objective. The reason is recorded, and required in
// spirit if not in schema: an objective stopped for a reason nobody wrote down
// is one nobody can decide to restart.
func (h *ReconcileHandler) Pause(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	reason := req.Reason
	if p, ok := auth.PrincipalFromContext(r.Context()); ok {
		if reason == "" {
			reason = "paused by " + p.ID
		} else {
			reason = reason + " (paused by " + p.ID + ")"
		}
	}
	if err := h.Reconcile.Pause(r.Context(), id, reason); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	h.writeState(w, r, id)
}

// Resume puts a paused objective back into rotation, clearing the failure and
// stall counters that stopped it. See featurereconcile.Service.Resume for why
// clearing them is the point rather than a convenience.
func (h *ReconcileHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := objective.ObjectiveID(chi.URLParam(r, "id"))
	if err := h.Reconcile.Resume(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	h.writeState(w, r, id)
}

func (h *ReconcileHandler) writeState(w http.ResponseWriter, r *http.Request, id objective.ObjectiveID) {
	st, err := h.Reconcile.State(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}
