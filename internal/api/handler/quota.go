package handler

import (
	"encoding/json"
	"net/http"
	"time"

	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// QuotaHandler exposes the limits in force and how much of them a twin has
// used.
//
// It is read-mostly on purpose. The one write is a reset, which is an operator
// override rather than an ordinary operation and is gated on quota:admin.
type QuotaHandler struct {
	Quota karakuriquota.Deps
}

// Config reports the limits in force, so an operator can see what a deployment
// is actually enforcing rather than inferring it from the config file it thinks
// is loaded.
//
// GET /api/v1/quota
func (h *QuotaHandler) Config(w http.ResponseWriter, _ *http.Request) {
	t := h.Quota.Tiers
	writeJSON(w, map[string]any{
		"request": map[string]any{
			"algorithm":  string(t.Request.Algorithm),
			"limit":      t.Request.Limit,
			"window":     t.Request.Window.String(),
			"per_second": t.Request.RatePerSecond(),
		},
		"capability":         quotaSummary(t.Capability.Cap, string(t.Capability.Period)),
		"llm_tokens":         quotaSummary(t.LLMTokens.Cap, string(t.LLMTokens.Period)),
		"adapter":            quotaSummary(t.Adapter.Cap, string(t.Adapter.Period)),
		"pressure_threshold": karakuriquota.PressureThreshold,
	})
}

func quotaSummary(cap int, period string) map[string]any {
	return map[string]any{"cap": cap, "period": period}
}

// Usage reports a twin's current consumption without spending any of it.
//
// GET /api/v1/quota/usage?twin=<id>
func (h *QuotaHandler) Usage(w http.ResponseWriter, r *http.Request) {
	twin := r.URL.Query().Get("twin")
	if twin == "" {
		http.Error(w, "twin is required", http.StatusBadRequest)
		return
	}
	usage, err := h.Quota.Usage(r.Context(), twin, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := map[string]any{}
	for tier, dec := range usage {
		out[tier] = map[string]any{
			"limit":     dec.Limit,
			"remaining": dec.Remaining,
			"used":      dec.Used(),
			"reset_at":  dec.ResetAt,
			"allowed":   dec.Allowed,
		}
	}
	writeJSON(w, map[string]any{"twin_id": twin, "tiers": out})
}

// Reset clears a twin's counters for the current period.
//
// It affects the period containing now and nothing else, so an override today
// cannot hand back yesterday's budget.
//
// POST /api/v1/quota/reset  {"twin": "...", "capability": "..."}
func (h *QuotaHandler) Reset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Twin       string `json:"twin"`
		Capability string `json:"capability"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "body must be JSON", http.StatusBadRequest)
		return
	}
	if body.Twin == "" {
		http.Error(w, "twin is required", http.StatusBadRequest)
		return
	}

	now := time.Now()
	if err := h.Quota.Reset(r.Context(), body.Twin, body.Capability, now); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"twin_id": body.Twin, "capability": body.Capability, "reset": true})
}
