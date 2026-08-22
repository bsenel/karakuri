package integration_test

import (
	"net/http"
	"testing"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// The standing-objective lifecycle over the real HTTP surface: declare it,
// read its control loop, stop it, start it again, and stop holding it.
//
// Against a real server and a real database rather than the service in
// isolation, because the parts that break in production are the ones between
// the layers — a route that is not registered, a validation that runs after the
// write, a state row the handler cannot find.
func TestStandingObjectiveLifecycle(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	twinID := createTwin(t, baseURL, adminToken, "standing-twin")
	objID := createObjective(t, baseURL, adminToken, twinID, "keep the build green")

	// An objective that has not been declared standing has no control loop,
	// and says so rather than inventing an empty one.
	resp := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+objID+"/reconcile", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("reconcile state on a one-shot objective = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	// Declare it.
	resp = doJSON(t, adminToken, http.MethodPut, baseURL+"/api/v1/objectives/"+objID+"/standing", map[string]any{
		"cadence": map[string]any{
			"sense":        "15m",
			"cron":         "0 8 * * 1-5",
			"timezone":     "Europe/Istanbul",
			"resync":       "24h",
			"min_interval": "10m",
			"quiet":        []string{"22:00-07:00"},
		},
		"autonomy": map[string]any{
			"level":         "propose",
			"ceiling":       "act_with_notice",
			"promote_after": 5,
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("declare = %d, want 200", resp.StatusCode)
	}
	state := decodeJSON(t, resp)
	if state["objective_id"] != objID {
		t.Errorf("state names objective %v, want %s", state["objective_id"], objID)
	}
	if state["autonomy"] != "propose" {
		t.Errorf("autonomy = %v, want propose", state["autonomy"])
	}
	// Freshly declared means due now: the first useful thing to tell somebody
	// who has written down a desired state is whether it already holds.
	if state["next_due_at"] == nil {
		t.Error("a freshly declared objective is not due")
	}

	// The objective itself carries the declaration.
	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+objID, nil)
	obj := decodeJSON(t, resp)
	if obj["mode"] != "standing" {
		t.Errorf("mode = %v, want standing", obj["mode"])
	}
	cadence, ok := obj["cadence"].(map[string]any)
	if !ok || cadence["timezone"] != "Europe/Istanbul" {
		t.Errorf("cadence = %v, want the one declared", obj["cadence"])
	}

	// Pause, and the reason is recorded with who did it.
	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives/"+objID+"/pause",
		map[string]any{"reason": "investigating a noisy adapter"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause = %d, want 200", resp.StatusCode)
	}
	state = decodeJSON(t, resp)
	if state["paused"] != true {
		t.Errorf("paused = %v, want true", state["paused"])
	}
	if reason, _ := state["paused_reason"].(string); reason == "" {
		t.Error("a pause with no recorded reason is one nobody can decide to undo")
	}

	// A paused objective refuses a manual reconcile rather than quietly
	// ignoring it.
	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives/"+objID+"/reconcile", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("reconcile on a paused objective = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// Resume clears what stopped it.
	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives/"+objID+"/resume", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume = %d, want 200", resp.StatusCode)
	}
	state = decodeJSON(t, resp)
	if state["paused"] != false {
		t.Errorf("paused = %v after resume, want false", state["paused"])
	}
	if state["consecutive_failures"] != float64(0) {
		t.Errorf("consecutive_failures = %v after resume, want 0", state["consecutive_failures"])
	}

	// Stop holding it. The objective survives; only the supervision stops.
	resp = doJSON(t, adminToken, http.MethodDelete, baseURL+"/api/v1/objectives/"+objID+"/standing", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("undeclare = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+objID+"/reconcile", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("reconcile state after undeclare = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+objID, nil)
	obj = decodeJSON(t, resp)
	if obj["title"] == nil {
		t.Error("the objective did not survive being undeclared")
	}
	if mode, _ := obj["mode"].(string); mode == "standing" {
		t.Error("mode is still standing after undeclare")
	}
}

// A cadence that will not parse is a standing objective that silently never
// runs, and that failure is invisible precisely because nothing happening is
// what it looks like when everything is fine. So it is refused at the door,
// and nothing is written.
func TestStandingDeclarationRejectsBadInput(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	twinID := createTwin(t, baseURL, adminToken, "standing-twin")
	objID := createObjective(t, baseURL, adminToken, twinID, "keep the build green")

	bad := []struct {
		name string
		body map[string]any
	}{
		{"two schedules", map[string]any{"cadence": map[string]any{"every": "1h", "cron": "0 8 * * *"}}},
		{"malformed cron", map[string]any{"cadence": map[string]any{"cron": "not a cron"}}},
		{"unknown timezone", map[string]any{"cadence": map[string]any{"timezone": "Mars/Olympus_Mons"}}},
		{"unparseable duration", map[string]any{"cadence": map[string]any{"sense": "fifteen minutes"}}},
		{"malformed quiet window", map[string]any{"cadence": map[string]any{"quiet": []string{"22:00"}}}},
		{"unknown autonomy level", map[string]any{"autonomy": map[string]any{"level": "full_send"}}},
		{"unknown ceiling", map[string]any{"autonomy": map[string]any{"ceiling": "superuser"}}},
	}

	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, adminToken, http.MethodPut,
				baseURL+"/api/v1/objectives/"+objID+"/standing", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("declare = %d, want 400", resp.StatusCode)
			}
		})
	}

	// Nothing was written by any of them.
	resp := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+objID+"/reconcile", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a rejected declaration left a control loop behind (%d)", resp.StatusCode)
	}
}

// Declaring is a heavier permission than reading, and pausing is deliberately
// lighter than declaring: at 3am the person who can see something is wrong is
// not always the one who was allowed to start it.
func TestStandingPermissionsAreSeparate(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	twinID := createTwin(t, baseURL, adminToken, "standing-twin")
	objID := createObjective(t, baseURL, adminToken, twinID, "keep the build green")
	viewer := createUser(t, baseURL, adminToken, "vera", karakuriauth.RoleViewer)

	resp := doJSON(t, viewer, http.MethodPut, baseURL+"/api/v1/objectives/"+objID+"/standing",
		map[string]any{"cadence": map[string]any{"sense": "15m"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a viewer declared a standing objective (%d)", resp.StatusCode)
	}
	resp.Body.Close()

	// An operator may.
	operator := createUser(t, baseURL, adminToken, "olive", karakuriauth.RoleOperator)
	resp = doJSON(t, operator, http.MethodPut, baseURL+"/api/v1/objectives/"+objID+"/standing",
		map[string]any{"cadence": map[string]any{"sense": "15m"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an operator could not declare a standing objective (%d)", resp.StatusCode)
	}
	resp.Body.Close()

	// And a viewer may read what it has been doing, but not stop it.
	resp = doJSON(t, viewer, http.MethodGet, baseURL+"/api/v1/objectives/"+objID+"/reconcile", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a viewer could not read the control loop (%d)", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, viewer, http.MethodPost, baseURL+"/api/v1/objectives/"+objID+"/pause", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a viewer paused a standing objective (%d)", resp.StatusCode)
	}
	resp.Body.Close()
}
