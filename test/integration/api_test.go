package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	objectivepkg "github.com/bsenel/karakuri/internal/core/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	domainsw "github.com/bsenel/karakuri/domains/software"
)

// startServer starts the API server on a random port and returns the base URL + cleanup func.
func startServer(t *testing.T) (baseURL string, cleanup func()) {
	t.Helper()

	// Create a temp SQLite file
	dbFile, err := os.CreateTemp("", "karakuri-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	dbPath := dbFile.Name()
	dbFile.Close()

	// Build config with empty auth token so no auth is required
	cfg := config.Default()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = dbPath
	cfg.Auth.Token = ""
	cfg.Git.RepoPath = t.TempDir()
	cfg.Git.WorktreeBase = "worktrees"
	cfg.Git.BaseBranch = "main"
	cfg.Git.BranchPrefix = "karakuri"
	cfg.Observability.Exporters = nil

	// Open DB and run migrations
	gormDB, err := platformdb.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(gormDB, cfg.Database.DSN); err != nil {
		os.Remove(dbPath)
		t.Fatalf("migrate db: %v", err)
	}
	store := storage.NewGORMStorage(gormDB)

	// LLM providers — Claude handles missing key gracefully (returns mock)
	providers := llm.NewRegistry(nil)
	claude, err := llm.NewClaudeProvider()
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("claude provider: %v", err)
	}
	providers.Register(claude)

	// Observability — empty registry is fine
	exporters := observability.NewExporterRegistry()
	otel := observability.NewOTel(exporters)

	// Git worktree manager against the temp dir
	wt, err := git.NewGoGitWorktreeManager(cfg.Git)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("worktree manager: %v", err)
	}

	toolReg := tools.NewRegistry()
	hub := event.NewHub()
	capReg := capability.NewRegistry()
	envReg := environment.NewRegistry()
	domReg := domain.NewRegistry()

	// Register universal capabilities
	for _, cap := range capability.Universal {
		_ = capReg.Register(cap)
	}

	// Register software domain pack (enabled)
	swPack := domainsw.New()
	ctx := context.Background()
	if err := domReg.Register(ctx, swPack, domain.Config{}); err != nil {
		t.Logf("domain register (non-fatal): %v", err)
	}
	for _, cap := range swPack.Capabilities() {
		_ = capReg.Register(cap)
	}
	for _, factory := range swPack.EnvironmentFactories() {
		_ = envReg.Register(factory)
	}
	var templates []objectivepkg.Template
	templates = append(templates, swPack.ObjectiveTemplates()...)

	apiApp := api.NewApp(cfg, store, providers, toolReg, exporters, wt, hub, otel, capReg, envReg, domReg, templates, nil, nil)

	// Listen on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := &http.Server{Handler: apiApp.Handler()}
	go func() { _ = srv.Serve(ln) }()

	base := "http://" + addr

	// Wait for server to be ready
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	return base, func() {
		_ = srv.Shutdown(context.Background())
		os.Remove(dbPath)
	}
}

// doJSON is a helper that issues an HTTP request with a JSON body and returns the response.
func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// decodeJSON decodes the response body into a map.
func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return m
}

// decodeJSONSlice decodes the response body into a slice of maps.
func decodeJSONSlice(t *testing.T, resp *http.Response) []any {
	t.Helper()
	defer resp.Body.Close()
	var s []any
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode json slice: %v", err)
	}
	return s
}

// assertStatus checks the response status code.
func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d: %s", want, resp.StatusCode, string(body))
	}
}

// assertField asserts that the map contains a key with a non-zero value.
func assertField(t *testing.T, m map[string]any, key string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("expected key %q in response, got keys: %v", key, mapKeys(m))
	}
	if v == nil || fmt.Sprintf("%v", v) == "" {
		t.Fatalf("expected non-empty value for key %q, got %v", key, v)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestHealth verifies GET /api/v1/health returns 200 with status: ok.
func TestHealth(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	resp, err := http.Get(baseURL + "/api/v1/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	m := decodeJSON(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", m["status"])
	}
}

// TestTwinCRUD tests POST /twins, GET /twins/:id, GET /twins, PUT /twins/:id.
func TestTwinCRUD(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	t.Run("create", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
			"name":   "test-twin",
			"kind":   "assistant",
			"domain": "software",
		})
		assertStatus(t, resp, http.StatusOK)
		m := decodeJSON(t, resp)
		assertField(t, m, "id")
		assertField(t, m, "name")
	})

	t.Run("create_then_get", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
			"name":   "twin-for-get",
			"kind":   "executor",
			"domain": "software",
		})
		assertStatus(t, resp, http.StatusOK)
		created := decodeJSON(t, resp)
		id := created["id"].(string)

		resp2 := doJSON(t, http.MethodGet, baseURL+"/api/v1/twins/"+id, nil)
		assertStatus(t, resp2, http.StatusOK)
		got := decodeJSON(t, resp2)
		if got["id"] != id {
			t.Fatalf("expected id=%s, got %v", id, got["id"])
		}
	})

	t.Run("list", func(t *testing.T) {
		resp := doJSON(t, http.MethodGet, baseURL+"/api/v1/twins", nil)
		assertStatus(t, resp, http.StatusOK)
		_ = decodeJSONSlice(t, resp)
	})

	t.Run("update", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
			"name":   "original-name",
			"kind":   "watcher",
			"domain": "software",
		})
		assertStatus(t, resp, http.StatusOK)
		created := decodeJSON(t, resp)
		id := created["id"].(string)

		resp2 := doJSON(t, http.MethodPut, baseURL+"/api/v1/twins/"+id, map[string]any{
			"name": "updated-name",
		})
		assertStatus(t, resp2, http.StatusOK)
		updated := decodeJSON(t, resp2)
		if updated["name"] != "updated-name" {
			t.Fatalf("expected name=updated-name, got %v", updated["name"])
		}
	})
}

// TestObjectiveCRUD tests POST /objectives, GET /objectives/:id, GET /objectives, POST /objectives/:id/status.
func TestObjectiveCRUD(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	// Create a twin first
	twinResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
		"name": "obj-twin", "kind": "assistant", "domain": "software",
	})
	assertStatus(t, twinResp, http.StatusOK)
	twinM := decodeJSON(t, twinResp)
	twinID := twinM["id"].(string)

	t.Run("create", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/objectives", map[string]any{
			"title":       "Test Objective",
			"description": "An integration test objective",
			"domain":      "software",
			"priority":    1,
			"twin_id":     twinID,
		})
		assertStatus(t, resp, http.StatusOK)
		m := decodeJSON(t, resp)
		assertField(t, m, "id")
		assertField(t, m, "title")
	})

	t.Run("create_then_get", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/objectives", map[string]any{
			"title":  "Objective for Get",
			"domain": "software",
		})
		assertStatus(t, resp, http.StatusOK)
		created := decodeJSON(t, resp)
		id := created["id"].(string)

		resp2 := doJSON(t, http.MethodGet, baseURL+"/api/v1/objectives/"+id, nil)
		assertStatus(t, resp2, http.StatusOK)
		got := decodeJSON(t, resp2)
		if got["id"] != id {
			t.Fatalf("expected id=%s, got %v", id, got["id"])
		}
	})

	t.Run("list", func(t *testing.T) {
		resp := doJSON(t, http.MethodGet, baseURL+"/api/v1/objectives", nil)
		assertStatus(t, resp, http.StatusOK)
		_ = decodeJSONSlice(t, resp)
	})

	t.Run("update_status", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/objectives", map[string]any{
			"title":  "Status Objective",
			"domain": "software",
		})
		assertStatus(t, resp, http.StatusOK)
		created := decodeJSON(t, resp)
		id := created["id"].(string)

		resp2 := doJSON(t, http.MethodPost, baseURL+"/api/v1/objectives/"+id+"/status", map[string]any{
			"status": "active",
		})
		assertStatus(t, resp2, http.StatusOK)
		m := decodeJSON(t, resp2)
		if m["status"] != "active" {
			t.Fatalf("expected status=active, got %v", m["status"])
		}
	})
}

// TestLoopStartStatus tests POST /loops (start a loop) and GET /loops/:id/status.
func TestLoopStartStatus(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	// Create twin and objective
	twinResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
		"name": "loop-twin", "kind": "executor", "domain": "software",
	})
	assertStatus(t, twinResp, http.StatusOK)
	twinM := decodeJSON(t, twinResp)
	twinID := twinM["id"].(string)

	objResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/objectives", map[string]any{
		"title": "Loop Objective", "domain": "software",
	})
	assertStatus(t, objResp, http.StatusOK)
	objM := decodeJSON(t, objResp)
	objID := objM["id"].(string)

	t.Run("start_loop", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/loops", map[string]any{
			"objective_id": objID,
			"twin_id":      twinID,
			"max_iter":     1,
		})
		assertStatus(t, resp, http.StatusOK)
		m := decodeJSON(t, resp)
		assertField(t, m, "loop_id")

		loopID := m["loop_id"].(string)

		// Poll status within 2s
		deadline := time.Now().Add(2 * time.Second)
		var statusM map[string]any
		for time.Now().Before(deadline) {
			statusResp := doJSON(t, http.MethodGet, baseURL+"/api/v1/loops/"+loopID+"/status", nil)
			if statusResp.StatusCode == http.StatusOK {
				statusM = decodeJSON(t, statusResp)
				break
			}
			statusResp.Body.Close()
			time.Sleep(50 * time.Millisecond)
		}
		if statusM == nil {
			t.Fatal("loop status never returned 200")
		}
		assertField(t, statusM, "loop_id")
	})
}

// TestMemoryStoreRecall tests POST /memory/store and POST /memory/recall.
func TestMemoryStoreRecall(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	t.Run("store", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/memory/store", map[string]any{
			"id":       "test-entry-1",
			"agent_id": "test-agent",
			"tier":     "episodic",
			"content":  "integration test memory entry",
			"confidence": 0.9,
		})
		assertStatus(t, resp, http.StatusOK)
		m := decodeJSON(t, resp)
		if m["status"] != "stored" {
			t.Fatalf("expected status=stored, got %v", m["status"])
		}
	})

	t.Run("recall", func(t *testing.T) {
		// Store first
		storeResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/memory/store", map[string]any{
			"id":         "recall-entry-1",
			"agent_id":   "recall-agent",
			"tier":       "episodic",
			"content":    "unique content for recall test",
			"confidence": 0.95,
		})
		assertStatus(t, storeResp, http.StatusOK)
		storeResp.Body.Close()

		// Recall
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/memory/recall", map[string]any{
			"agent_id": "recall-agent",
			"tiers":    []string{"episodic"},
			"top_k":    5,
		})
		assertStatus(t, resp, http.StatusOK)
		entries := decodeJSONSlice(t, resp)
		if len(entries) == 0 {
			t.Fatal("expected at least one memory entry, got none")
		}
	})
}

// TestArtifactWriteGet tests POST /artifacts and GET /artifacts/:sha.
func TestArtifactWriteGet(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	t.Run("write", func(t *testing.T) {
		resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/artifacts", map[string]any{
			"objective_id": "obj-test-123",
			"agent_id":     "agent-test",
			"capability":   "software.act.write_code",
			"content":      "package main\n\nfunc main() {}\n",
		})
		assertStatus(t, resp, http.StatusOK)
		m := decodeJSON(t, resp)
		assertField(t, m, "sha")
	})

	t.Run("write_then_get", func(t *testing.T) {
		content := "# Integration test artifact content"
		writeResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/artifacts", map[string]any{
			"objective_id": "obj-get-test",
			"agent_id":     "agent-get",
			"capability":   "software.act.write_design_doc",
			"content":      content,
		})
		assertStatus(t, writeResp, http.StatusOK)
		m := decodeJSON(t, writeResp)
		sha := m["sha"].(string)

		getResp := doJSON(t, http.MethodGet, baseURL+"/api/v1/artifacts/"+sha, nil)
		assertStatus(t, getResp, http.StatusOK)
		body, err := io.ReadAll(getResp.Body)
		getResp.Body.Close()
		if err != nil {
			t.Fatalf("read artifact body: %v", err)
		}
		if string(body) != content {
			t.Fatalf("expected content %q, got %q", content, string(body))
		}
	})
}

// TestCheckpointList tests GET /checkpoints.
func TestCheckpointList(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()

	// GET /checkpoints should return empty list initially (no error)
	resp := doJSON(t, http.MethodGet, baseURL+"/api/v1/checkpoints", nil)
	assertStatus(t, resp, http.StatusOK)
	items := decodeJSONSlice(t, resp)
	// May be empty — that's fine
	_ = items
}
