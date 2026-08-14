package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurereport "github.com/bsenel/karakuri/internal/feature/report"
	"github.com/go-chi/chi/v5"
)

// ReportHandler is the digest surface: declare a schedule, see what it will
// produce before committing to it, send one now, delete it.
type ReportHandler struct {
	Reports *featurereport.Service
}

// Create declares a report schedule.
func (h *ReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TwinID        string            `json:"twin_id"`
		Cadence       objective.Cadence `json:"cadence"`
		Channel       string            `json:"channel"`
		Instance      string            `json:"instance"`
		Target        string            `json:"target"`
		Window        string            `json:"window"`
		SendWhenEmpty bool              `json:"send_when_empty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sch, err := h.Reports.Declare(r.Context(), digest.Schedule{
		TwinID:        req.TwinID,
		Cadence:       req.Cadence,
		Channel:       req.Channel,
		Instance:      req.Instance,
		Target:        req.Target,
		Window:        req.Window,
		SendWhenEmpty: req.SendWhenEmpty,
	})
	if err != nil {
		// Every failure from Declare is a malformed declaration — an unknown
		// channel, a cadence that will not parse, a window that is not a
		// duration. None of them is the server's fault.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, sch)
}

// List returns schedules, optionally for one twin.
func (h *ReportHandler) List(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.Reports.List(r.Context(), r.URL.Query().Get("twin_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, schedules)
}

func (h *ReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.Reports.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Send delivers one schedule's digest now, outside its cadence.
//
// Synchronous, unlike a manual reconcile: a digest is a handful of queries and
// one message, so the caller can be told whether it actually went out. A 202
// here would mean "we will try", and the most common reason to press this
// button is to find out whether delivery works at all.
func (h *ReportHandler) Send(w http.ResponseWriter, r *http.Request) {
	if err := h.Reports.Send(r.Context(), chi.URLParam(r, "id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

// Preview assembles and renders a digest without delivering it, so somebody
// can see what a schedule will produce before committing a daily mail to it.
func (h *ReportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	twinID := r.URL.Query().Get("twin_id")
	if twinID == "" {
		http.Error(w, "twin_id is required", http.StatusBadRequest)
		return
	}
	window := 24 * time.Hour
	if raw := r.URL.Query().Get("window"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			http.Error(w, "window must be a positive duration", http.StatusBadRequest)
			return
		}
		window = d
	}
	d, err := h.Reports.Preview(r.Context(), twinID, window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, d)
}
