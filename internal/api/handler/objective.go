package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type ObjectiveHandler struct {
	Objectives *featureobj.Service

	// Scopes narrows a listing to the objectives the caller may read, the same
	// way TwinHandler.Scopes does. An objective inherits its twin's containers,
	// so a grant on an organisation reaches the work being done in it.
	Scopes karakuriauth.ScopeAuthorizer
}

func (h *ObjectiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		Domain            string   `json:"domain"`
		AdditionalDomains []string `json:"additional_domains"`
		Priority          int      `json:"priority"`
		MaxIterations     int      `json:"max_iterations"`
		TwinID            string   `json:"twin_id"`
		TemplateID        string   `json:"template_id"`

		SuccessCriteria []objective.Criterion `json:"success_criteria"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	obj, err := h.Objectives.Create(r.Context(), featureobj.CreateRequest{
		Title:             req.Title,
		Description:       req.Description,
		Domain:            req.Domain,
		AdditionalDomains: req.AdditionalDomains,
		Priority:          req.Priority,
		MaxIterations:     req.MaxIterations,
		TwinID:            req.TwinID,
		TemplateID:        req.TemplateID,
		SuccessCriteria:   req.SuccessCriteria,
	})
	var invalid *featureobj.ValidationError
	if errors.As(err, &invalid) {
		writeFieldError(w, invalid.Field, invalid.Err)
		return
	}
	if errors.Is(err, featureobj.ErrNoRequiredCriterion) {
		// The caller's mistake, and one they can fix: name a template whose
		// criteria include a required one.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, obj)
}

func (h *ObjectiveHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	obj, err := h.Objectives.Get(r.Context(), objective.ObjectiveID(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, obj)
}

// List returns the objectives the caller may read. See TwinHandler.List for why
// the filter lives in the handler and why it is a narrowing rather than the
// authority.
func (h *ObjectiveHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	visible, hidden, err := karakuriauth.ListFor(
		r.Context(), h.Scopes, principal.ID, karakuriauth.ActionObjectiveRead, "objective")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	objs, err := h.Objectives.List(r.Context(), storage.ObjectiveFilter{
		TwinID:  r.URL.Query().Get("twin_id"),
		Status:  r.URL.Query().Get("status"),
		Visible: visible,
		Hidden:  hidden,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, objs)
}

func (h *ObjectiveHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Objectives.UpdateStatus(r.Context(), objective.ObjectiveID(id), objective.ObjectiveStatus(req.Status)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": req.Status})
}

// SetBaseline serves PUT /objectives/{id}/criteria/{criterion_id}/baseline
// with the body {"value": <number>}: a person setting a measured criterion's
// baseline (Phase 41 step 7 part D2). The person in the audit row is the
// authenticated caller, never a body field.
func (h *ObjectiveHandler) SetBaseline(w http.ResponseWriter, r *http.Request) {
	setCriterionValue(w, r, h.Objectives.SetBaseline)
}

// SetTarget serves PUT /objectives/{id}/criteria/{criterion_id}/target with
// the body {"value": <number>}: a person changing a measured criterion's
// target, which restarts its sustain window (Phase 41 step 8). The person in
// the audit row is the authenticated caller, never a body field.
func (h *ObjectiveHandler) SetTarget(w http.ResponseWriter, r *http.Request) {
	setCriterionValue(w, r, h.Objectives.SetTarget)
}

// setCriterionValue reads {"value": <number>}, hands it to set with the
// criterion named in the path and the authenticated caller, and answers the
// objective, a 400 naming the field, or a 404.
func setCriterionValue(w http.ResponseWriter, r *http.Request,
	set func(context.Context, objective.ObjectiveID, string, float64, string) (objective.Objective, error),
) {
	var req struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var value float64
	if len(req.Value) == 0 || string(req.Value) == "null" {
		writeFieldError(w, "value", errors.New("value is required"))
		return
	}
	if err := json.Unmarshal(req.Value, &value); err != nil {
		// A string, or a number out of float64's range.
		writeFieldError(w, "value", errors.New("value must be a finite number"))
		return
	}
	setBy := ""
	if p, ok := auth.PrincipalFromContext(r.Context()); ok {
		setBy = p.ID
	}
	obj, err := set(r.Context(), objective.ObjectiveID(chi.URLParam(r, "id")),
		chi.URLParam(r, "criterion_id"), value, setBy)
	var invalid *featureobj.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeFieldError(w, invalid.Field, invalid.Err)
	case errors.Is(err, coreerrors.ErrObjectiveNotFound), errors.Is(err, featureobj.ErrCriterionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		writeJSON(w, obj)
	}
}

// writeFieldError is a 400 in JSON naming the field to change, so a client
// reads it without parsing prose.
func writeFieldError(w http.ResponseWriter, field string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "field": field})
}

func (h *ObjectiveHandler) ListTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.Objectives.ListTemplates())
}
