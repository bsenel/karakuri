package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// watch opens the global stream and returns the event types and twin IDs it
// delivers, stopping when the context is done.
//
// It reads with a scanner rather than an SSE client because the wire format is
// two lines and a blank one, and the thing under test is which events arrive —
// not the framing.
func watch(t *testing.T, ctx context.Context, baseURL, token string) <-chan map[string]any {
	t.Helper()
	out := make(chan map[string]any, 32)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("stream = %d, want 200", resp.StatusCode)
	}

	go func() {
		defer resp.Body.Close()
		defer close(out)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var evt map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err != nil {
				continue
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// collect drains a stream for a fixed window and returns what arrived. The
// window is generous because the assertion that matters is about what is
// *absent*, and a short one would pass for the wrong reason.
func collect(ch <-chan map[string]any, window time.Duration) []map[string]any {
	var got []map[string]any
	deadline := time.After(window)
	for {
		select {
		case evt, open := <-ch:
			if !open {
				return got
			}
			got = append(got, evt)
		case <-deadline:
			return got
		}
	}
}

// Phase 17's property, restated for a live stream: a watcher in one tenant sees
// their own org's events and none of the other's.
//
// This is the case the endpoint could most easily have got wrong. A listing that
// leaks is one bad response; a stream that leaks keeps leaking for as long as
// the browser tab is open.
func TestGlobalStreamIsConfinedToTheCallersTenant(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	oliveToken := createScopedUser(t, baseURL, adminToken, "olive",
		karakuriauth.RoleOperator, acmeEng.Label())

	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	events := watch(t, streamCtx, baseURL, oliveToken)
	// Give the subscription a moment to reach the hub; a publish that races the
	// Subscribe is dropped, which would make this pass for the wrong reason.
	time.Sleep(200 * time.Millisecond)

	// Twin updates publish with the twin's ID on them, which is what the filter
	// resolves through the tenancy tree.
	for _, id := range []string{acmeTwin, globexTwin} {
		resp := doJSON(t, adminToken, http.MethodPut, baseURL+"/api/v1/twins/"+id,
			map[string]any{"state": map[string]any{"note": "touched"}})
		resp.Body.Close()
	}

	got := collect(events, 1500*time.Millisecond)
	sawAcme, sawGlobex := false, false
	for _, evt := range got {
		switch evt["twin_id"] {
		case acmeTwin:
			sawAcme = true
		case globexTwin:
			sawGlobex = true
		}
	}
	if !sawAcme {
		t.Errorf("olive saw none of her own team's events; got %d events", len(got))
	}
	if sawGlobex {
		t.Error("olive saw the other tenant's events on the global stream")
	}
}

// An administrator holding the wildcard watches the whole deployment, so the
// filter did not become a second denial path.
func TestGlobalStreamShowsEverythingToAnUnrestrictedReader(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	events := watch(t, streamCtx, baseURL, adminToken)
	time.Sleep(200 * time.Millisecond)

	for _, id := range []string{acmeTwin, globexTwin} {
		resp := doJSON(t, adminToken, http.MethodPut, baseURL+"/api/v1/twins/"+id,
			map[string]any{"state": map[string]any{"note": "touched"}})
		resp.Body.Close()
	}

	seen := map[string]bool{}
	for _, evt := range collect(events, 1500*time.Millisecond) {
		if id, ok := evt["twin_id"].(string); ok {
			seen[id] = true
		}
	}
	for _, id := range []string{acmeTwin, globexTwin} {
		if !seen[id] {
			t.Errorf("the administrator did not see events for %s", id)
		}
	}
}
