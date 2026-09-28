package handler

import (
	"context"
	"net/http"

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
}

// Calibrate scores the judge against resolved checkpoints in a window.
//
// POST /api/v1/eval/calibrate {"twin":…,"since":RFC3339,"until":RFC3339,"limit":…}
func (h *EvalHandler) Calibrate(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
