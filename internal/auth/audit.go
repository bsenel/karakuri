package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// AuditDenial returns an Enforcer OnDeny hook that records refused requests in
// the same tool_events table as authority-bounds escalations (Phase 13).
//
// Attempts belong next to approvals: an operator reviewing who approved what
// should see, in one place, who was turned away. `krk audit --kind
// authz_denied` and the /audit page pick these up with no new endpoint.
//
// The decision trace goes into the payload — the matched policy, the granting
// role, the binding scope and each condition result — so a denial can be
// explained after the fact rather than re-derived.
func AuditDenial(store storage.StorageAdapter) func(*http.Request, auth.Principal, auth.Decision) {
	return func(r *http.Request, p auth.Principal, d auth.Decision) {
		payload, err := json.Marshal(map[string]any{
			"method":   r.Method,
			"path":     r.URL.Path,
			"action":   d.Action,
			"resource": d.Resource,
			"decision": d,
		})
		if err != nil {
			slog.Warn("could not encode authorization denial for audit", "err", err)
			return
		}
		event := storage.ToolEvent{
			ID:               newEventID(),
			AgentID:          p.ID,
			Capability:       string(d.Action),
			Success:          false,
			Kind:             storage.ToolEventAuthzDenied,
			EscalationReason: d.Reason,
			PayloadJSON:      string(payload),
			CreatedAt:        storage.Now(),
		}
		// A failed audit write must not turn a 403 into a 500 — the request is
		// already being refused, and losing the record is the lesser problem.
		if err := store.SaveToolEvent(r.Context(), event); err != nil {
			slog.Warn("could not record authorization denial", "principal", p.ID, "action", d.Action, "err", err)
		}
	}
}

func newEventID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
