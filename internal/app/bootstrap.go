package app

import (
	"log/slog"
	"os"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Bootstrap struct {
	Config    *config.Config
	App       *api.App
	Store     storage.StorageAdapter
	Worktrees git.WorktreeManager
}

func BootstrapServer(cfgPath string) (*Bootstrap, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Warn("config load failed, using defaults", "err", err)
		cfg = config.Default()
	}
	gormDB, err := db.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		return nil, err
	}
	store := storage.NewGORMStorage(gormDB)

	providers := llm.NewRegistry(cfg.Providers.Fallback)
	claude, err := llm.NewClaudeProvider()
	if err != nil {
		return nil, err
	}
	providers.Register(claude)
	providers.Register(llm.NewGeminiProvider())
	providers.Register(llm.NewCursorProvider())
	providers.Register(llm.NewCopilotProvider())

	exporters := observability.NewExporterRegistry()
	for _, ec := range cfg.Observability.Exporters {
		if !ec.Enabled {
			continue
		}
		switch ec.Name {
		case "local":
			mfmt, lfmt := "ndjson", "ndjson"
			if ec.Formats != nil {
				if v, ok := ec.Formats["metrics"]; ok {
					mfmt = v
				}
				if v, ok := ec.Formats["logs"]; ok {
					lfmt = v
				}
			}
			exporters.Register(observability.NewLocalFileExporter(ec.Path, mfmt, lfmt))
		case "aws":
			exporters.Register(observability.NewAWSExporter())
		}
	}
	otel := observability.NewOTel(exporters)

	wt, err := git.NewGoGitWorktreeManager(cfg.Git)
	if err != nil {
		return nil, err
	}

	toolReg := tools.NewRegistry()
	hub := event.NewHub()

	apiApp := api.NewApp(cfg, store, providers, toolReg, exporters, wt, hub, otel)
	return &Bootstrap{Config: cfg, App: apiApp, Store: store, Worktrees: wt}, nil
}

func ConfigPath() string {
	if p := os.Getenv("KARAKURI_CONFIG"); p != "" {
		return p
	}
	return "config/default.yaml"
}
