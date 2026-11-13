package command

import "testing"

// A standing objective that has not reconciled yet comes back with a null
// history; the pretty format says so and names the command that runs a pass.
func TestReconcileStatusSaysWhenNoPassHasRun(t *testing.T) {
	want := "No reconcile passes yet. Run one now with: krk objective reconcile obj_123\n"
	for _, body := range []string{`{"state":{"objective_id":"obj_123"},"history":null}`, `{"state":{},"history":[]}`} {
		apiURL := startListServer(t, body)
		_, stderr, err := runKrkSplit("--api-url", apiURL, "objective", "reconcile-status", "obj_123")
		if err != nil {
			t.Fatalf("krk objective reconcile-status: %v", err)
		}
		if stderr != want {
			t.Errorf("reconcile-status on %s wrote %q to stderr, want %q", body, stderr, want)
		}
	}

	apiURL := startListServer(t, `{"state":{},"history":[{"outcome":"sensed"}]}`)
	_, stderr, err := runKrkSplit("--api-url", apiURL, "objective", "reconcile-status", "obj_123")
	if err != nil {
		t.Fatalf("krk objective reconcile-status: %v", err)
	}
	if stderr != "" {
		t.Errorf("a history with a pass wrote %q to stderr, want nothing", stderr)
	}

	apiURL = startListServer(t, `{"state":{},"history":null}`)
	_, stderr, err = runKrkSplit("--api-url", apiURL, "--output", "json", "objective", "reconcile-status", "obj_123")
	if err != nil {
		t.Fatalf("krk --output json objective reconcile-status: %v", err)
	}
	if stderr != "" {
		t.Errorf("--output json wrote %q to stderr, want nothing", stderr)
	}
}
