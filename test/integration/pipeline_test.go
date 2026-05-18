//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/app"
)

func TestStrategyPipeline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	cfg := config.Default()
	cfg.Database.DSN = filepath.Join(dir, "test.db")
	cfg.Git.RepoPath = dir
	cfg.WorkflowsDir = "../../workflows"
	data, _ := os.ReadFile("../../config/default.yaml")
	_ = os.WriteFile(cfgPath, data, 0o644)
	os.Setenv("KARAKURI_CONFIG", cfgPath)

	boot, err := app.BootstrapServer(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(boot.App.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"mode": "strategy", "input": "test idea"})
	resp, err := http.Post(srv.URL+"/api/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var sess map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()
	sha, _ := sess["sha"].(string)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/sessions/"+sha+"/run", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := http.Get(srv.URL + "/api/v1/sessions/" + sha + "/status")
		var st map[string]string
		_ = json.NewDecoder(r.Body).Decode(&st)
		r.Body.Close()
		if st["state"] == "completed" {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("session did not complete")
}
