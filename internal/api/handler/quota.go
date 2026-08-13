package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
	"github.com/go-chi/chi/v5"
)

// QuotaHandler exposes the limits in force and how much of them a twin has
// used.
//
// It is read-mostly on purpose. The one write is a reset, which is an operator
// override rather than an ordinary operation and is gated on quota:admin.
type QuotaHandler struct {
	Quota karakuriquota.Deps

	// Authorizer answers the two checks that depend on a request's body rather
	// than its URL: whether an approval is for a subject the caller holds, and
	// which containers a cost report may total.
	Authorizer auth.Authorizer

	// Scopes narrows a cost report to what the caller may see, the same way it
	// narrows the twin listing.
	Scopes karakuriauth.ScopeAuthorizer

	// Containers resolves a subject to the containers it sits in, so an org
	// administrator can approve for a twin inside their org.
	Containers karakuriauth.ScopeResolver
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

// Requests, decisions and cost reporting (Phase 18).
//
// Two authorization rules live here rather than in the route table, because
// both depend on what the request names in its body:
//
//   - Approving a request means holding the scope it would raise, which is the
//     same rule `krk auth bindings add` follows: you cannot grant yourself
//     something you do not already have.
//   - A cost report is filtered to the containers the reader may see, from the
//     same bindings the per-resource check reads. Without it the report is a
//     way around the tenancy the twin listing enforces.

// SubmitRequest records a request for more of a tier.
//
// POST /api/v1/quota/requests
func (h *QuotaHandler) SubmitRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tier      string `json:"tier"`
		Twin      string `json:"twin"`
		Principal string `json:"principal"`
		Cap       int    `json:"cap"`
		Window    string `json:"window"`
		Until     string `json:"until"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return
	}

	// Asking on your own behalf is the common case, so an unnamed principal
	// means the caller. Asking on somebody else's is allowed — a team lead
	// requesting for a service account — and is what the approver sees.
	subject := body.Principal
	if subject == "" && body.Twin == "" {
		subject = principal.ID
	}

	window, err := parseOptionalDuration(body.Window)
	if err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "window: "+err.Error())
		return
	}
	until, err := parseOptionalTime(body.Until)
	if err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "until: "+err.Error())
		return
	}

	req, err := h.Quota.Submit(r.Context(), karakuriquota.SubmitRequest{
		Tier: body.Tier, PrincipalID: subject, TwinID: body.Twin,
		Cap: body.Cap, Window: window, ExpiresAt: until,
		Reason: body.Reason, RequestedBy: principal.ID,
	})
	if err != nil {
		writeQuotaError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, req)
}

// ListRequests returns matching requests, narrowed to the rows the caller may
// see.
//
// The route says whether you may list; this says which rows, the same shape the
// twin listing uses. A row is visible if you asked for it or if you could
// approve it — so a viewer sees their own requests, an approver scoped to an
// organisation sees that organisation's, and nobody reads another tenant's
// reasons for wanting more.
//
// GET /api/v1/quota/requests?status=pending&twin=<id>&mine=true
func (h *QuotaHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	q := r.URL.Query()

	f := quota.RequestFilter{Status: quota.RequestStatus(q.Get("status"))}
	if twin := q.Get("twin"); twin != "" {
		f.Subject = karakuriquota.TwinKey(twin)
	}
	if q.Get("mine") == "true" {
		f.RequestedBy = principal.ID
	}

	requests, err := h.Quota.ListRequests(r.Context(), f)
	if err != nil {
		writeQuotaError(w, err)
		return
	}

	// One authorization per row. A quota request list is bounded by its filter
	// and short in practice, and the alternative — asking once whether the
	// caller is an approver "in general" — has no answer once approval is
	// scoped: there is no such thing as approving in general.
	visible := make([]quota.Request, 0, len(requests))
	for _, req := range requests {
		if req.RequestedBy == principal.ID {
			visible = append(visible, req)
			continue
		}
		allowed, _, err := karakuriauth.MayActOn(
			r.Context(), h.Authorizer, h.Containers, principal,
			karakuriauth.ActionQuotaApprove, karakuriquota.ScopeForSubject(req.Subject))
		if err != nil {
			http.Error(w, "authorization could not be evaluated", http.StatusInternalServerError)
			return
		}
		if allowed {
			visible = append(visible, req)
		}
	}
	writeJSON(w, visible)
}

// Decide approves or rejects a request.
//
// Approving requires holding the scope the request would raise. Without that,
// the permission to approve is the permission to raise anybody's limit —
// including your own, in a tenant you have no claim on.
//
// POST /api/v1/quota/requests/{id}/decide  {"approve": true, "note": "..."}
func (h *QuotaHandler) Decide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || h.Authorizer == nil {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return
	}

	id := chi.URLParam(r, "id")
	req, err := h.Quota.Requests().Store.GetRequest(r.Context(), id)
	if err != nil {
		writeQuotaError(w, err)
		return
	}

	// The subject a request would raise is a resource like any other, carrying
	// the containers it sits in — so an org administrator can approve for a
	// twin inside their org and nobody else's. The route gate answered "may you
	// approve at all"; this answers "may you approve this one", which it cannot,
	// because the subject arrives inside the stored request rather than the URL.
	//
	// Only approval is checked. Rejecting is refusing to raise a limit, and
	// somebody who may decide at all can always decline.
	if body.Approve {
		allowed, reason, err := karakuriauth.MayActOn(
			r.Context(), h.Authorizer, h.Containers, principal,
			karakuriauth.ActionQuotaApprove, karakuriquota.ScopeForSubject(req.Subject))
		if err != nil {
			http.Error(w, "authorization could not be evaluated", http.StatusInternalServerError)
			return
		}
		if !allowed {
			authError(w, http.StatusForbidden, "forbidden",
				"you can only approve a raise for a subject you already hold: "+reason)
			return
		}
	}

	decided, err := h.Quota.Decide(r.Context(), id, principal.ID, body.Note, body.Approve)
	if err != nil {
		writeQuotaError(w, err)
		return
	}
	writeJSON(w, decided)
}

// CostReport answers what was spent, filtered to what the caller may see.
//
// GET /api/v1/cost?since=…&until=…&group_by=provider,day&twin=…&limit=…
func (h *QuotaHandler) CostReport(w http.ResponseWriter, r *http.Request) {
	if !h.Quota.Costs.Enabled() {
		writeJSON(w, []any{})
		return
	}
	q := r.URL.Query()

	query := cost.Query{Limit: atoiOr(q.Get("limit"), 0)}
	var err error
	if query.Since, err = parseOptionalTime(q.Get("since")); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "since: "+err.Error())
		return
	}
	if query.Until, err = parseOptionalTime(q.Get("until")); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "until: "+err.Error())
		return
	}
	for _, g := range splitCSV(q.Get("group_by")) {
		query.GroupBy = append(query.GroupBy, cost.GroupBy(g))
	}
	if twin := q.Get("twin"); twin != "" {
		query.Subjects = append(query.Subjects, karakuriquota.CostSubject(twin))
	}
	query.Providers = splitCSV(q.Get("provider"))
	// A label the caller asked for narrows the report; it never widens it. The
	// tenancy filter below intersects with it rather than replacing it, so
	// naming another tenant's org returns nothing instead of their spend.
	asked := splitCSV(q.Get("label"))

	// The tenancy filter. Spend is attributed to the containers a resource sat
	// in, so the same scope set that decides which twins a caller may list
	// decides which spend they may total — otherwise a report is a way around
	// the isolation Phase 17 built.
	principal, _ := auth.PrincipalFromContext(r.Context())
	visible, _, err := karakuriauth.ListFor(
		r.Context(), h.Scopes, principal.ID, karakuriauth.ActionTwinRead, "twin")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if visible != nil {
		if visible.Empty() {
			// No grants means no rows, rather than every row.
			writeJSON(w, []any{})
			return
		}
		if len(visible.Labels) == 0 && len(visible.LabelPrefixes) == 0 {
			// A caller whose only grants name individual twins can be answered
			// exactly, by naming those twins as subjects.
			for _, id := range visible.IDs {
				query.Subjects = append(query.Subjects, karakuriquota.CostSubject(id))
			}
			if len(query.Subjects) == 0 {
				writeJSON(w, []any{})
				return
			}
		} else {
			query.Labels = visible.Labels
			if len(asked) > 0 {
				query.Labels = intersectLabels(visible.Labels, asked)
				if len(query.Labels) == 0 {
					// Asked for a container they cannot see.
					writeJSON(w, []any{})
					return
				}
			}
		}
	} else if len(asked) > 0 {
		// An unrestricted reader gets exactly what they asked for.
		query.Labels = asked
	}

	buckets, err := h.Quota.CostReport(r.Context(), query)
	if err != nil {
		writeQuotaError(w, err)
		return
	}
	writeJSON(w, buckets)
}

// intersectLabels keeps only the labels present in both, which is how a
// caller's own filter narrows a scoped report without being able to escape it.
func intersectLabels(visible, asked []string) []string {
	var out []string
	for _, a := range asked {
		for _, v := range visible {
			if a == v {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

func writeQuotaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, quota.ErrRequestNotFound):
		authError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, quota.ErrRequestDecided):
		authError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, quota.ErrInvalidPolicy), errors.Is(err, cost.ErrInvalidEvent):
		authError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		// A tier that does not exist, a subject with nothing to name it: both
		// are the caller's mistake and both are worth reading.
		authError(w, http.StatusBadRequest, "bad_request", err.Error())
	}
}

func parseOptionalDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

func parseOptionalTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func atoiOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}
