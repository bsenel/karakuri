package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type HealthHandler struct {
	Providers  *llm.Registry
	Tools      *tools.Registry
	Exporters  *observability.ExporterRegistry
	Worktrees  git.WorktreeManager
	RepoPath   string
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "ok",
		"providers":  h.Providers.All(),
		"adapters":   h.Tools.Summary(),
		"exporters":  h.Exporters.Names(),
		"git":        map[string]any{"repo_path": h.RepoPath, "worktree_manager": h.Worktrees != nil},
	})
}
