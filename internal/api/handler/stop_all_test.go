package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// Phase 37 step 4: POST /stop-all with {"reason": ...}.
//
// The route carries no route-level permission (see
// internal/auth/stop_all_test.go). The handler answers reconcile's StopAll
// predicate from the caller's own permissions, run by run: loop:stop on
// auth.Resource("loop", id) for a loop and objective:pause on
// auth.Resource("objective", id) for an objective, each with its containers
// attached as the per-run routes attach them. The answer is StopAll's result:
// loops_stopped, objectives_paused and not_reached.
//
// The world is the acceptance line's: two running one-shot loops and two
// standing objectives.

// stopAllWorld is the deployment the stop-all tests run against.
type stopAllWorld struct {
	loops *stoppingLoops
	store storage.StorageAdapter
	rec   *featurereconcile.Service
	authz *auth.StoreAuthorizer
	auths auth.Store
}

const (
	stopAllObjA  = "obj-a"
	stopAllObjB  = "obj-b"
	stopAllLoop1 = "loop-1"
	stopAllLoop2 = "loop-2"
)

func newStopAllWorld(t *testing.T) *stopAllWorld {
	t.Helper()
	loops := newStoppingLoops()
	loops.found[stopAllLoop1] = featureloop.StopFoundRunning
	loops.found[stopAllLoop2] = featureloop.StopFoundRunning
	store, rec := newStopReconcile(t, loops)

	ctx := context.Background()
	for _, id := range []objective.ObjectiveID{stopAllObjA, stopAllObjB} {
		obj := objective.Objective{
			ID:       id,
			Mode:     objective.ModeStanding,
			Domain:   "software",
			Cadence:  &objective.Cadence{Sense: "30s"},
			Autonomy: &objective.Autonomy{Level: objective.AutonomySense, Ceiling: objective.AutonomySense},
		}
		if err := store.SaveObjective(ctx, obj); err != nil {
			t.Fatalf("save objective %s: %v", id, err)
		}
		if err := rec.Declare(ctx, obj); err != nil {
			t.Fatalf("declare %s: %v", id, err)
		}
	}
	for i, id := range []string{stopAllLoop1, stopAllLoop2} {
		if err := store.SaveLoopState(ctx, loop.State{
			LoopID:      id,
			ObjectiveID: objective.ObjectiveID("obj-oneshot-" + string(rune('1'+i))),
			Status:      objective.StatusActive,
		}); err != nil {
			t.Fatalf("save loop state %s: %v", id, err)
		}
	}

	auths := auth.NewMemoryStore()
	for _, r := range karakuriauth.BuiltinRoles() {
		if err := auths.PutRole(ctx, r); err != nil {
			t.Fatalf("put role %s: %v", r.Name, err)
		}
	}
	return &stopAllWorld{loops: loops, store: store, rec: rec, authz: auth.NewAuthorizer(auths), auths: auths}
}

// grant binds principal to a new role holding policies, under scope ("" for
// everywhere).
func (w *stopAllWorld) grant(t *testing.T, principal, scope string, policies ...auth.Policy) {
	t.Helper()
	ctx := context.Background()
	role := auth.Role{Name: "role-" + principal, Policies: policies}
	if err := w.auths.PutRole(ctx, role); err != nil {
		t.Fatalf("put role: %v", err)
	}
	w.bind(t, principal, role.Name, scope)
}

func (w *stopAllWorld) bind(t *testing.T, principal, role, scope string) {
	t.Helper()
	ctx := context.Background()
	if err := w.auths.PutPrincipal(ctx, auth.Principal{ID: principal}); err != nil {
		t.Fatalf("put principal: %v", err)
	}
	if err := w.auths.PutBinding(ctx, auth.RoleBinding{ID: "b-" + principal + "-" + role, PrincipalID: principal, Role: role, Scope: scope}); err != nil {
		t.Fatalf("put binding: %v", err)
	}
}

// post sends body to POST /api/v1/stop-all as principal, on the path the
// server mounts, through a handler wired with the world's authorizer and
// containers.
func (w *stopAllWorld) post(t *testing.T, principal, body string, containers karakuriauth.ScopeLookup) *httptest.ResponseRecorder {
	t.Helper()
	h := &handler.LoopHandler{
		Loop:       w.loops,
		Store:      w.store,
		Reconcile:  w.rec,
		Authorizer: w.authz,
		Containers: containers,
	}
	r := chi.NewRouter()
	r.Post("/api/v1"+karakuriauth.StopAllPath, h.StopAll)

	req := httptest.NewRequest(http.MethodPost, "/api/v1"+karakuriauth.StopAllPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: principal}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// stopAllAnswer is the response body, decoded with the field names the
// OpenAPI schema documents.
type stopAllAnswer struct {
	LoopsStopped []struct {
		LoopID string `json:"loop_id"`
		Found  string `json:"found"`
	} `json:"loops_stopped"`
	ObjectivesPaused []struct {
		ObjectiveID   string `json:"objective_id"`
		PassCancelled bool   `json:"pass_cancelled"`
	} `json:"objectives_paused"`
	NotReached []struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Reason string `json:"reason"`
	} `json:"not_reached"`
}

func decodeStopAll(t *testing.T, rec *httptest.ResponseRecorder) stopAllAnswer {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json; body %s", ct, rec.Body.String())
	}
	// The three lists are always present, as arrays, so "nothing" reads as an
	// empty list and never as a missing field.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	for _, key := range []string{"loops_stopped", "objectives_paused", "not_reached"} {
		v, ok := raw[key]
		if !ok {
			t.Fatalf("answer has no %q; body %s", key, rec.Body.String())
		}
		if !strings.HasPrefix(strings.TrimSpace(string(v)), "[") {
			t.Fatalf("%q = %s, want a list; body %s", key, v, rec.Body.String())
		}
	}
	var got stopAllAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return got
}

func (a stopAllAnswer) stopped() []string {
	var out []string
	for _, l := range a.LoopsStopped {
		out = append(out, l.LoopID)
	}
	sort.Strings(out)
	return out
}

func (a stopAllAnswer) paused() []string {
	var out []string
	for _, o := range a.ObjectivesPaused {
		out = append(out, o.ObjectiveID)
	}
	sort.Strings(out)
	return out
}

// notReached lists "kind id" for each run not reached, sorted, and fails on
// one whose reason is blank.
func (a stopAllAnswer) notReached(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, n := range a.NotReached {
		if n.Reason == "" {
			t.Errorf("not reached %s %s carries no reason", n.Kind, n.ID)
		}
		out = append(out, n.Kind+" "+n.ID)
	}
	sort.Strings(out)
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func (w *stopAllWorld) requirePaused(t *testing.T, id objective.ObjectiveID, want bool) {
	t.Helper()
	st, err := w.store.GetReconcileState(context.Background(), id)
	if err != nil {
		t.Fatalf("get reconcile state %s: %v", id, err)
	}
	if st.Paused != want {
		t.Errorf("objective %s paused = %t, want %t", id, st.Paused, want)
	}
}

func (w *stopAllWorld) stopCalls() []string {
	var out []string
	for _, c := range w.loops.calls {
		out = append(out, c.loopID)
	}
	sort.Strings(out)
	return out
}

var allNotReached = []string{
	"loop " + stopAllLoop1, "loop " + stopAllLoop2,
	"objective " + stopAllObjA, "objective " + stopAllObjB,
}

// A caller who may stop and pause everything stops both loops and pauses both
// objectives, with the reason given and as the authenticated principal.
func TestStopAllByAnAdminStopsAndPausesEverything(t *testing.T) {
	w := newStopAllWorld(t)
	w.bind(t, "alice", karakuriauth.RoleAdmin, "")

	got := decodeStopAll(t, w.post(t, "alice", `{"reason":"pushing to the wrong remote","principal":"mallory"}`, nil))

	if want := []string{stopAllLoop1, stopAllLoop2}; !sameStrings(got.stopped(), want) {
		t.Errorf("loops_stopped = %v, want %v", got.stopped(), want)
	}
	if want := []string{stopAllObjA, stopAllObjB}; !sameStrings(got.paused(), want) {
		t.Errorf("objectives_paused = %v, want %v", got.paused(), want)
	}
	if nr := got.notReached(t); len(nr) != 0 {
		t.Errorf("not_reached = %v, want none for a caller who may stop everything", nr)
	}
	for _, c := range w.loops.calls {
		if c.principal != "alice" || c.reason != "pushing to the wrong remote" {
			t.Errorf("Stop(%s) as %q for %q, want the authenticated alice and the given reason", c.loopID, c.principal, c.reason)
		}
	}
	if want := []string{stopAllLoop1, stopAllLoop2}; !sameStrings(w.stopCalls(), want) {
		t.Errorf("Stop called for %v, want %v", w.stopCalls(), want)
	}
	w.requirePaused(t, stopAllObjA, true)
	w.requirePaused(t, stopAllObjB, true)
	st, err := w.store.GetReconcileState(context.Background(), stopAllObjA)
	if err != nil {
		t.Fatalf("get reconcile state: %v", err)
	}
	if !strings.Contains(st.PausedReason, "pushing to the wrong remote") {
		t.Errorf("paused reason = %q, want it to carry the stop-all's reason", st.PausedReason)
	}
}

// The acceptance line's narrow caller: allowed to pause obj-a and nothing
// else, it pauses obj-a only, and the answer names every other run as not
// reached, so it is never told "everything" when it was not.
func TestStopAllByACallerWhoMayPauseOneObjectivePausesThatOneOnly(t *testing.T) {
	w := newStopAllWorld(t)
	w.grant(t, "pat", "", auth.Allow("pause-obj-a", karakuriauth.ActionObjectivePause, "objective:"+stopAllObjA))

	got := decodeStopAll(t, w.post(t, "pat", `{"reason":"obj-a is misbehaving"}`, nil))

	if want := []string{stopAllObjA}; !sameStrings(got.paused(), want) {
		t.Errorf("objectives_paused = %v, want %v", got.paused(), want)
	}
	if len(got.LoopsStopped) != 0 {
		t.Errorf("loops_stopped = %v, want none: pat holds no loop:stop", got.stopped())
	}
	want := []string{"loop " + stopAllLoop1, "loop " + stopAllLoop2, "objective " + stopAllObjB}
	if nr := got.notReached(t); !sameStrings(nr, want) {
		t.Errorf("not_reached = %v, want %v", nr, want)
	}
	if len(w.loops.calls) != 0 {
		t.Errorf("Stop called for %v, want no loop stopped", w.stopCalls())
	}
	w.requirePaused(t, stopAllObjA, true)
	w.requirePaused(t, stopAllObjB, false)
}

// A caller scoped to a container holds objective:pause only on what is in it.
// The handler attaches each run's containers as the per-run routes do, so a
// team-scoped pauser reaches the objective in its team and nothing else.
func TestStopAllAsksAboutTheSameScopedRefsAsThePerRunRoutes(t *testing.T) {
	w := newStopAllWorld(t)
	w.grant(t, "tess", "container:team-a", auth.Allow("pause-any", karakuriauth.ActionObjectivePause, "objective:*"))
	containers := stopAllScopes{"objective/" + stopAllObjA: {"container:team-a"}}

	got := decodeStopAll(t, w.post(t, "tess", `{"reason":"team-a stops its own"}`, containers))

	if want := []string{stopAllObjA}; !sameStrings(got.paused(), want) {
		t.Errorf("objectives_paused = %v, want %v: obj-a is in team-a", got.paused(), want)
	}
	want := []string{"loop " + stopAllLoop1, "loop " + stopAllLoop2, "objective " + stopAllObjB}
	if nr := got.notReached(t); !sameStrings(nr, want) {
		t.Errorf("not_reached = %v, want %v", nr, want)
	}
	w.requirePaused(t, stopAllObjB, false)
}

// loop:stop is asked on each loop by its own ID: a caller allowed to stop
// loop-1 stops loop-1, and loop-2 is not reached.
func TestStopAllAsksLoopStopOnEachLoop(t *testing.T) {
	w := newStopAllWorld(t)
	w.grant(t, "lou", "", auth.Allow("stop-loop-1", karakuriauth.ActionLoopStop, "loop:"+stopAllLoop1))

	got := decodeStopAll(t, w.post(t, "lou", `{"reason":"loop-1 is on the wrong branch"}`, nil))

	if want := []string{stopAllLoop1}; !sameStrings(got.stopped(), want) {
		t.Errorf("loops_stopped = %v, want %v", got.stopped(), want)
	}
	if want := []string{stopAllLoop1}; !sameStrings(w.stopCalls(), want) {
		t.Errorf("Stop called for %v, want %v", w.stopCalls(), want)
	}
	want := []string{"loop " + stopAllLoop2, "objective " + stopAllObjA, "objective " + stopAllObjB}
	if nr := got.notReached(t); !sameStrings(nr, want) {
		t.Errorf("not_reached = %v, want %v", nr, want)
	}
	w.requirePaused(t, stopAllObjA, false)
	w.requirePaused(t, stopAllObjB, false)
}

// A caller holding neither loop:stop nor objective:pause changes nothing, and
// the answer says every run was not reached. Holding other permissions (a
// viewer's reads) does not stand in for either.
func TestStopAllByACallerWithNeitherPermissionChangesNothing(t *testing.T) {
	w := newStopAllWorld(t)
	w.bind(t, "vera", karakuriauth.RoleViewer, "")

	got := decodeStopAll(t, w.post(t, "vera", `{"reason":"just looking"}`, nil))

	if len(got.LoopsStopped) != 0 || len(got.ObjectivesPaused) != 0 {
		t.Errorf("stopped %v and paused %v, want nothing", got.stopped(), got.paused())
	}
	if nr := got.notReached(t); !sameStrings(nr, allNotReached) {
		t.Errorf("not_reached = %v, want %v", nr, allNotReached)
	}
	if len(w.loops.calls) != 0 {
		t.Errorf("Stop called for %v, want none", w.stopCalls())
	}
	w.requirePaused(t, stopAllObjA, false)
	w.requirePaused(t, stopAllObjB, false)
}

// A missing reason is filled in from the principal, as POST /loops/{id}/stop
// fills it, so the record still says who.
func TestStopAllWithoutAReasonIsFilledInFromThePrincipal(t *testing.T) {
	w := newStopAllWorld(t)
	w.bind(t, "alice", karakuriauth.RoleAdmin, "")

	decodeStopAll(t, w.post(t, "alice", `{}`, nil))

	if len(w.loops.calls) == 0 {
		t.Fatal("Stop was not called")
	}
	for _, c := range w.loops.calls {
		if c.reason != "stopped by alice" {
			t.Errorf("Stop(%s) reason = %q, want %q", c.loopID, c.reason, "stopped by alice")
		}
	}
}

// stopAllScopes is a container lookup keyed "type/id".
type stopAllScopes map[string][]string

func (s stopAllScopes) ScopesOf(_ context.Context, resourceType, resourceID string) ([]string, error) {
	return s[resourceType+"/"+resourceID], nil
}
