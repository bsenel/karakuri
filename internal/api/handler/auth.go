package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// AuthHandler serves login, token rotation, and the principal/role/policy
// surface behind the auth:* permissions.
//
// Two kinds of client authenticate here, and they need different things:
//
//   - API clients (krk, CI) want the tokens in the response body, to store
//     wherever they keep credentials.
//   - Browsers must not be handed a token at all. Anything reachable from
//     JavaScript is readable by injected script, so the SPA asks for cookie
//     mode and the tokens go out as httpOnly cookies it can never read.
//
// The client says which it wants with `"cookie": true` on the request. Cookie
// mode omits the tokens from the response body entirely — handing them over
// and asking the caller not to store them would defeat the point.
type AuthHandler struct {
	Store      auth.Store
	Tokens     *auth.TokenService
	Authorizer *auth.StoreAuthorizer
	Catalog    *auth.Catalog
	Cookies    auth.CookieConfig

	// Containers resolves a binding scope to the container it names, so
	// granting can be bounded by containment (Phase 17). Nil on a deployment
	// with no tenancy tree, where a scope names a resource directly and the
	// check still works.
	Containers karakuriauth.ScopeResolver
}

// mayGrant enforces that a caller can only hand out a scope they already hold,
// writing the refusal itself and reporting whether the request should continue.
//
// Without it the permission to manage bindings is the permission to manage
// every tenant: an administrator scoped to one organisation could write
// themselves a binding over another, and the tree would be decoration.
func (h *AuthHandler) mayGrant(w http.ResponseWriter, r *http.Request, scope string) bool {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return false
	}
	allowed, reason, err := karakuriauth.MayGrant(r.Context(), h.Authorizer, h.Containers, principal, scope)
	if err != nil {
		http.Error(w, "authorization could not be evaluated", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		authError(w, http.StatusForbidden, "forbidden", reason)
		return false
	}
	return true
}

// sessionResponse is what cookie-mode clients get back: enough to know the
// login worked and when to expect expiry, and no credential.
type sessionResponse struct {
	TokenType string `json:"token_type"`
	ExpiresIn int    `json:"expires_in"`
}

// respondWithPair writes a token pair in whichever form the client asked for.
func (h *AuthHandler) respondWithPair(w http.ResponseWriter, r *http.Request, pair auth.TokenPair, useCookies bool) {
	if !useCookies {
		writeJSON(w, pair)
		return
	}
	h.Cookies.SetSession(w, r, pair)
	writeJSON(w, sessionResponse{TokenType: pair.TokenType, ExpiresIn: pair.ExpiresIn})
}

// Token exchanges a password for an access/refresh pair.
//
// POST /api/v1/auth/token  {"id": "...", "password": "..."}
func (h *AuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		Password string `json:"password"`
		Cookie   bool   `json:"cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	pair, err := h.Tokens.IssueForPassword(r.Context(), body.ID, body.Password)
	if err != nil {
		// Every failure mode — unknown principal, wrong password, disabled
		// account, service account with no password — is reported identically
		// so the endpoint cannot be used to enumerate accounts.
		//
		// The submitted ID is deliberately not logged. This endpoint is
		// unauthenticated, so that string is attacker-controlled, and writing
		// it verbatim lets anyone forge lines in a log stream that is shipped
		// to Datadog/Loki/Elasticsearch. The remote address is enough to spot
		// a brute-force attempt.
		slog.Info("login rejected", "remote", karakuriauth.SanitizeLogValue(r.RemoteAddr))
		authError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}
	h.respondWithPair(w, r, pair, body.Cookie)
}

// Refresh rotates a refresh token, returning a new pair.
//
// POST /api/v1/auth/refresh  {"refresh_token": "..."}
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	// An empty body is normal in cookie mode — the credential is the cookie.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	token, useCookies := body.RefreshToken, false
	if token == "" {
		// Cookie mode is inferred rather than declared here: a browser cannot
		// put the token in the body, because it cannot read the cookie.
		token, useCookies = h.Cookies.Refresh(r), true
	}
	pair, err := h.Tokens.IssueForRefresh(r.Context(), token)
	if err != nil {
		if useCookies {
			// The cookie is spent or revoked; clear it so the browser stops
			// replaying a credential the server will keep rejecting.
			h.Cookies.ClearSession(w, r)
		}
		if errors.Is(err, auth.ErrRefreshTokenReuse) {
			// The family is already revoked by the time we get here. Say so
			// plainly: the holder needs to re-authenticate, and somebody should
			// look at why a spent token was replayed.
			slog.Warn("refresh token reuse detected — family revoked", "remote", karakuriauth.SanitizeLogValue(r.RemoteAddr))
			authError(w, http.StatusUnauthorized, "token_reuse", "refresh token was already used; all sessions in its family have been revoked")
			return
		}
		authError(w, http.StatusUnauthorized, "invalid_refresh_token", err.Error())
		return
	}
	// Rotation means the old cookie is now spent: it has to be replaced in the
	// same response, or the browser is left holding a dead token.
	h.respondWithPair(w, r, pair, useCookies)
}

// Revoke invalidates the presented refresh token's whole family — "log out".
//
// POST /api/v1/auth/revoke  {"refresh_token": "..."}
func (h *AuthHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	token := body.RefreshToken
	if token == "" {
		token = h.Cookies.Refresh(r)
	}
	// The local session ends either way: a browser that cannot reach the server
	// must still end up logged out, so the cookies are cleared before the
	// revocation result is known.
	h.Cookies.ClearSession(w, r)
	if err := h.Tokens.Revoke(r.Context(), token); err != nil {
		authError(w, http.StatusUnauthorized, "invalid_refresh_token", err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "revoked"})
}

// Me returns the calling principal, its roles, and the concrete permissions
// those roles expand to. It is what a UI reads to decide which navigation to
// render.
//
// GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		authError(w, http.StatusUnauthorized, "unauthorized", "no principal")
		return
	}
	roles, err := h.Authorizer.Roles(r.Context(), p.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	permissions, err := h.Authorizer.ExpandGrants(r.Context(), p.ID, h.Catalog)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bindings, err := h.Store.ListBindings(r.Context(), p.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"principal":   p,
		"roles":       roles,
		"permissions": permissions,
		"bindings":    bindings,
	})
}

// ListUsers returns every principal.
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	principals, err := h.Store.ListPrincipals(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, principals)
}

// CreateUser adds a principal, binds it to roles, and issues its first
// credential.
//
// A user gets a password; a service account gets a refresh token returned once
// in the response and never again, since only its hash is stored.
//
// POST /api/v1/auth/users
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		Roles    []string          `json:"roles"`
		Scope    string            `json:"scope"`
		Password string            `json:"password"`
		Service  bool              `json:"service_account"`
		Attrs    map[string]string `json:"attrs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		authError(w, http.StatusBadRequest, "bad_request", "id is required")
		return
	}
	if !body.Service && body.Password == "" {
		authError(w, http.StatusBadRequest, "bad_request", "password is required (or set service_account)")
		return
	}

	// The scope is checked before the principal is written, so a refused
	// request leaves nothing behind.
	scope := body.Scope
	if scope == "" {
		scope = "*"
	}
	if len(body.Roles) > 0 && !h.mayGrant(w, r, scope) {
		return
	}

	kind := auth.KindUser
	if body.Service {
		kind = auth.KindService
	}
	principal := auth.Principal{ID: body.ID, Name: body.Name, Kind: kind, Attrs: body.Attrs}
	if err := h.Store.PutPrincipal(r.Context(), principal); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, role := range body.Roles {
		if _, err := h.Store.GetRole(r.Context(), role); err != nil {
			authError(w, http.StatusBadRequest, "unknown_role", err.Error())
			return
		}
		binding := auth.RoleBinding{
			ID:          body.ID + ":" + role + ":" + scope,
			PrincipalID: body.ID,
			Role:        role,
			Scope:       scope,
		}
		if err := h.Store.PutBinding(r.Context(), binding); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	resp := map[string]any{"principal": principal, "roles": body.Roles, "scope": scope}
	if body.Service {
		pair, err := h.Tokens.IssueForPrincipal(r.Context(), body.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Shown once. Only the hash is stored, so it cannot be recovered.
		resp["refresh_token"] = pair.RefreshToken
		resp["access_token"] = pair.AccessToken
	} else if err := h.Tokens.SetPassword(r.Context(), body.ID, body.Password); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, resp)
}

// CreateBinding grants a principal a role over a scope.
//
// POST /api/v1/auth/bindings
func (h *AuthHandler) CreateBinding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PrincipalID string `json:"principal_id"`
		Role        string `json:"role"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	if body.PrincipalID == "" || body.Role == "" {
		authError(w, http.StatusBadRequest, "bad_request", "principal_id and role are required")
		return
	}
	if _, err := h.Store.GetPrincipal(r.Context(), body.PrincipalID); err != nil {
		authError(w, http.StatusBadRequest, "unknown_principal", err.Error())
		return
	}
	if _, err := h.Store.GetRole(r.Context(), body.Role); err != nil {
		authError(w, http.StatusBadRequest, "unknown_role", err.Error())
		return
	}
	scope := body.Scope
	if scope == "" {
		scope = "*"
	}
	if !h.mayGrant(w, r, scope) {
		return
	}
	binding := auth.RoleBinding{
		ID:          body.PrincipalID + ":" + body.Role + ":" + scope,
		PrincipalID: body.PrincipalID,
		Role:        body.Role,
		Scope:       scope,
	}
	if err := h.Store.PutBinding(r.Context(), binding); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, binding)
}

// ListRoles returns the role catalog with each role's flattened policies.
func (h *AuthHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.Store.ListRoles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, roles)
}

// ListPolicies returns either every role's policies, or — with ?principal=<id> —
// the effective grants reaching one principal.
func (h *AuthHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	if principalID := r.URL.Query().Get("principal"); principalID != "" {
		grants, err := h.Authorizer.Grants(r.Context(), principalID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, grants)
		return
	}
	roles, err := h.Store.ListRoles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var out []map[string]any
	for _, role := range roles {
		for _, p := range role.Policies {
			out = append(out, map[string]any{"role": role.Name, "policy": p})
		}
	}
	writeJSON(w, out)
}

// Check answers "may this principal do this?" and returns the full decision
// trace, so a policy can be debugged without probing endpoints.
//
// POST /api/v1/auth/check  {"principal":"vera","action":"loop:start","resource":"loop:*"}
func (h *AuthHandler) Check(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Principal string `json:"principal"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		Owner     string `json:"owner"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	principal, err := h.Store.GetPrincipal(r.Context(), body.Principal)
	if err != nil {
		authError(w, http.StatusBadRequest, "unknown_principal", err.Error())
		return
	}
	if !h.Catalog.Has(auth.Action(body.Action)) {
		authError(w, http.StatusBadRequest, "unknown_action", "action is not in the catalog")
		return
	}

	typ, id, _ := strings.Cut(body.Resource, ":")
	if id == "*" {
		id = ""
	}
	ref := auth.Resource(typ, id)
	if body.Owner != "" {
		ref = ref.WithOwner(body.Owner)
	}

	decision, err := h.Authorizer.Authorize(r.Context(), principal, auth.Action(body.Action), ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, decision)
}

// ListCatalog returns every registered action with its description.
func (h *AuthHandler) ListCatalog(w http.ResponseWriter, _ *http.Request) {
	type entry struct {
		Action      auth.Action `json:"action"`
		Description string      `json:"description"`
	}
	actions := h.Catalog.Actions()
	out := make([]entry, 0, len(actions))
	for _, a := range actions {
		desc, _ := h.Catalog.Describe(a)
		out = append(out, entry{Action: a, Description: desc})
	}
	writeJSON(w, out)
}

func authError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "reason": reason})
}
