// Command server is a self-contained demonstration of the auth module: a
// login endpoint, a rotating refresh endpoint, and two protected routes with
// different permission requirements.
//
// It imports nothing outside the standard library and this module. Run it and
// follow the printed transcript:
//
//	go run ./examples/server
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/jwt"
)

func main() {
	ctx := context.Background()
	store := auth.NewMemoryStore()

	// 1. Describe the actions this application recognises. A policy naming an
	//    action that is not here is rejected at seed time.
	catalog := auth.NewCatalog()
	catalog.MustRegister("note:read", "read a note")
	catalog.MustRegister("note:write", "create or edit a note")
	catalog.MustRegister("admin:manage", "administer the deployment")

	// 2. Define roles. Editor inherits reader; admin inherits editor but is
	//    explicitly denied write access to notes owned by someone else.
	roles := []auth.Role{
		{Name: "reader", System: true, Policies: []auth.Policy{
			auth.Allow("reader-read", "note:read", "note:*"),
		}},
		{Name: "editor", System: true, Inherits: []string{"reader"}, Policies: []auth.Policy{
			// Conditional: an editor may only write notes they own.
			auth.Allow("editor-write-own", "note:write", "note:*").
				When(auth.Condition{Kind: auth.CondOwnerEquals}),
		}},
		{Name: "admin", System: true, Inherits: []string{"editor"}, Policies: []auth.Policy{
			auth.Allow("admin-all", "*", "*"),
		}},
	}
	for _, r := range roles {
		if err := catalog.ValidateRole(r); err != nil {
			log.Fatalf("role %s: %v", r.Name, err)
		}
		if err := store.PutRole(ctx, r); err != nil {
			log.Fatal(err)
		}
	}

	// 3. Create principals and bind roles to them. Bob's binding is scoped to a
	//    single note, so his editor role does not reach anything else.
	must(store.PutPrincipal(ctx, auth.Principal{ID: "alice", Name: "Alice", Kind: auth.KindUser}))
	must(store.PutPrincipal(ctx, auth.Principal{ID: "bob", Name: "Bob", Kind: auth.KindUser}))
	must(store.PutBinding(ctx, auth.RoleBinding{ID: "b1", PrincipalID: "alice", Role: "editor"}))
	must(store.PutBinding(ctx, auth.RoleBinding{ID: "b2", PrincipalID: "bob", Role: "editor", Scope: "note:n1"}))

	// 4. Wire token issuance. There is no default signing key on purpose.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	key, err := jwt.NewHMACKey("k1", secret)
	must(err)
	keyring, err := jwt.NewKeyring(key)
	must(err)
	tokens, err := auth.NewTokenService(store, store, keyring, auth.TokenConfig{
		Issuer: "example", Audience: "example-api",
	})
	must(err)
	must(tokens.SetPassword(ctx, "alice", "hunter2"))
	must(tokens.SetPassword(ctx, "bob", "hunter2"))

	// 5. Build the router. Notes are owned, so the resource function reports an
	//    owner and the owner_equals condition can do its work.
	notes := map[string]string{"n1": "alice", "n2": "bob"}
	noteResource := func(r *http.Request) auth.ResourceRef {
		id := strings.TrimPrefix(r.URL.Path, "/notes/")
		return auth.Resource("note", id).WithOwner(notes[id])
	}

	authorizer := auth.NewAuthorizer(store)
	authenticated := auth.Authenticate(auth.NewJWTResolver(tokens, ""))

	mux := http.NewServeMux()
	mux.Handle("POST /login", loginHandler(tokens))
	mux.Handle("POST /refresh", refreshHandler(tokens))
	mux.Handle("GET /notes/{id}", authenticated(
		auth.RequirePermission(authorizer, "note:read", noteResource)(noteHandler("read")),
	))
	mux.Handle("PUT /notes/{id}", authenticated(
		auth.RequirePermission(authorizer, "note:write", noteResource)(noteHandler("wrote")),
	))

	// 6. Drive it, so `go run ./examples/server` shows the behaviour rather
	//    than just offering a port to curl.
	srv := httptest.NewServer(mux)
	defer srv.Close()
	transcript(srv.URL, authorizer, catalog)
}

func loginHandler(tokens *auth.TokenService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ ID, Password string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		pair, err := tokens.IssueForPassword(r.Context(), body.ID, body.Password)
		if err != nil {
			http.Error(w, `{"error":"invalid_credentials"}`, http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, pair)
	})
}

func refreshHandler(tokens *auth.TokenService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		pair, err := tokens.IssueForRefresh(r.Context(), body.RefreshToken)
		if err != nil {
			// Reuse of a spent token has already revoked the family by now.
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pair)
	})
}

func noteHandler(verb string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromContext(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{
			"result": fmt.Sprintf("%s %s note %s", p.Name, verb, r.PathValue("id")),
		})
	})
}

func transcript(base string, authorizer *auth.StoreAuthorizer, catalog *auth.Catalog) {
	ctx := context.Background()

	fmt.Println("== login ==")
	pair := login(base, "alice", "hunter2")
	fmt.Printf("alice: access token expires in %ds\n\n", pair.ExpiresIn)

	fmt.Println("== authorization ==")
	fmt.Printf("GET  /notes/n1 (alice owns it)  -> %s\n", call(base, http.MethodGet, "/notes/n1", pair.AccessToken))
	fmt.Printf("PUT  /notes/n1 (alice owns it)  -> %s\n", call(base, http.MethodPut, "/notes/n1", pair.AccessToken))
	fmt.Printf("PUT  /notes/n2 (bob owns it)    -> %s\n", call(base, http.MethodPut, "/notes/n2", pair.AccessToken))
	fmt.Printf("GET  /notes/n1 (no token)       -> %s\n\n", call(base, http.MethodGet, "/notes/n1", ""))

	fmt.Println("== scoped binding: bob is an editor on note:n1 only ==")
	bob := login(base, "bob", "hunter2")
	fmt.Printf("GET  /notes/n1 (in scope)       -> %s\n", call(base, http.MethodGet, "/notes/n1", bob.AccessToken))
	fmt.Printf("GET  /notes/n2 (out of scope)   -> %s\n\n", call(base, http.MethodGet, "/notes/n2", bob.AccessToken))

	fmt.Println("== refresh rotation ==")
	rotated := refresh(base, pair.RefreshToken)
	fmt.Printf("refresh #1                      -> new refresh token issued (%t)\n", rotated.RefreshToken != pair.RefreshToken)
	fmt.Printf("replay the spent token          -> %s\n", refreshRaw(base, pair.RefreshToken))
	fmt.Printf("use the rotated token after     -> %s\n\n", refreshRaw(base, rotated.RefreshToken))

	fmt.Println("== explaining a decision ==")
	d, err := authorizer.Authorize(ctx, auth.Principal{ID: "alice"}, "note:write", auth.Resource("note", "n2").WithOwner("bob"))
	must(err)
	fmt.Printf("alice -> note:write on note:n2  -> allowed=%t\n  reason: %s\n", d.Allowed, d.Reason)
	for _, c := range d.Conditions {
		fmt.Printf("  condition %s: satisfied=%t (%s)\n", c.Condition.Kind, c.Satisfied, c.Detail)
	}
	perms, err := authorizer.ExpandGrants(ctx, "alice", catalog)
	must(err)
	fmt.Printf("alice's effective permissions: %v\n", perms)
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func login(base, id, password string) tokenPair {
	body := strings.NewReader(fmt.Sprintf(`{"id":%q,"password":%q}`, id, password))
	resp, err := http.Post(base+"/login", "application/json", body)
	must(err)
	defer resp.Body.Close()
	var pair tokenPair
	must(json.NewDecoder(resp.Body).Decode(&pair))
	return pair
}

func refresh(base, token string) tokenPair {
	resp, err := http.Post(base+"/refresh", "application/json", strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, token)))
	must(err)
	defer resp.Body.Close()
	var pair tokenPair
	must(json.NewDecoder(resp.Body).Decode(&pair))
	return pair
}

func refreshRaw(base, token string) string {
	resp, err := http.Post(base+"/refresh", "application/json", strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, token)))
	must(err)
	defer resp.Body.Close()
	var body map[string]any
	must(json.NewDecoder(resp.Body).Decode(&body))
	if msg, ok := body["error"]; ok {
		return fmt.Sprintf("%d %v", resp.StatusCode, msg)
	}
	return fmt.Sprintf("%d ok", resp.StatusCode)
}

func call(base, method, path, token string) string {
	req, err := http.NewRequest(method, base+path, nil)
	must(err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	must(err)
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if reason, ok := body["reason"]; ok {
		return fmt.Sprintf("%d %v", resp.StatusCode, reason)
	}
	if result, ok := body["result"]; ok {
		return fmt.Sprintf("%d %v", resp.StatusCode, result)
	}
	return fmt.Sprintf("%d", resp.StatusCode)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
