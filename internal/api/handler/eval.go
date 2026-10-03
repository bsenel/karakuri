package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/feature/eval"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// evalCalibrator is the slice of eval.Service the calibrate route calls.
type evalCalibrator interface {
	Calibrate(ctx context.Context, f storage.ResolvedCheckpointFilter) (eval.CalibrationReport, error)
}

// EvalHandler serves judge calibration (Phase 30).
type EvalHandler struct {
	Calibrator evalCalibrator
	// Scopes narrows calibration to the twins the caller may read, the way it
	// narrows a cost report. Nil leaves it unscoped.
	Scopes karakuriauth.ScopeAuthorizer
}

type calibrateRequest struct {
	Twin  string `json:"twin"`
	Since string `json:"since"`
	Until string `json:"until"`
	Limit int    `json:"limit"`
}

// Calibrate scores the judge against resolved checkpoints in a window.
//
// POST /api/v1/eval/calibrate {"twin":…,"since":RFC3339,"until":RFC3339,"limit":…}
func (h *EvalHandler) Calibrate(w http.ResponseWriter, r *http.Request) {
	if h.Calibrator == nil {
		authError(w, http.StatusServiceUnavailable, "unavailable", "no judge provider is configured")
		return
	}
	var req calibrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		authError(w, http.StatusBadRequest, "bad_request", "body: "+err.Error())
		return
	}
	f := storage.ResolvedCheckpointFilter{TwinID: req.Twin, Limit: req.Limit}
	var err error
	if f.Since, err = parseOptionalTime(req.Since); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "since: "+err.Error())
		return
	}
	if f.Until, err = parseOptionalTime(req.Until); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "until: "+err.Error())
		return
	}

	// The tenancy filter, as the cost report applies it: the twins a caller
	// may list are the twins whose checkpoints they may calibrate against.
	principal, _ := auth.PrincipalFromContext(r.Context())
	visible, hidden, err := karakuriauth.ListFor(
		r.Context(), h.Scopes, principal.ID, karakuriauth.ActionTwinRead, "twin")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if f.TwinID != "" && slices.Contains(hidden.IDs, f.TwinID) {
		authError(w, http.StatusForbidden, "forbidden", "twin "+f.TwinID+" is not one you may read")
		return
	}
	if visible != nil {
		if visible.Empty() {
			// No grants means no checkpoints, rather than every checkpoint.
			writeJSON(w, eval.CalibrationReport{Since: f.Since, Until: f.Until})
			return
		}
		// The filter carries one twin and no containers, so a restricted
		// caller is answered only for a twin granted to them by id.
		switch {
		case f.TwinID != "" && !slices.Contains(visible.IDs, f.TwinID):
			authError(w, http.StatusForbidden, "forbidden", "twin "+f.TwinID+" is not one you may read")
			return
		case f.TwinID == "" && len(visible.IDs) == 1 && len(visible.Labels) == 0 && len(visible.LabelPrefixes) == 0:
			f.TwinID = visible.IDs[0]
		case f.TwinID == "":
			authError(w, http.StatusForbidden, "forbidden", "your grants are scoped: name a twin you may read")
			return
		}
	}

	report, err := h.Calibrator.Calibrate(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}
