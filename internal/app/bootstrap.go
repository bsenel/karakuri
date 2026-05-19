package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	objectivepkg "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	domainagri "github.com/bsenel/karakuri/domains/agriculture"
	domainconsult "github.com/bsenel/karakuri/domains/consulting"
	domainhc "github.com/bsenel/karakuri/domains/healthcare"
	domainlegal "github.com/bsenel/karakuri/domains/legal"
	domainmech "github.com/bsenel/karakuri/domains/mechanical"
	domainsw "github.com/bsenel/karakuri/domains/software"
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
	if err := db.RunMigrations(gormDB, cfg.Database.DSN); err != nil {
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

	capReg := capability.NewRegistry()
	envReg := environment.NewRegistry()
	domReg := domain.NewRegistry()

	ctx := context.Background()

	// Register universal capabilities
	for _, cap := range capability.Universal {
		_ = capReg.Register(cap)
	}

	// Register domain packs
	allPacks := []domain.Pack{
		domainsw.New(),
		domainagri.New(),
		domainconsult.New(),
		domainhc.New(),
		domainlegal.New(),
		domainmech.New(),
	}
	enabledDomains := make(map[string]config.DomainConfig)
	for _, dc := range cfg.Domains {
		enabledDomains[dc.ID] = dc
	}
	var allTemplates []objectivepkg.Template
	for _, pack := range allPacks {
		dc, ok := enabledDomains[pack.ID()]
		if !ok || !dc.Enabled {
			// Register stub packs as disabled
			_ = domReg.Register(ctx, pack, domain.Config{})
			continue
		}
		if err := domReg.Register(ctx, pack, domain.Config(dc.Options)); err != nil {
			slog.Warn("domain pack init failed", "domain", pack.ID(), "err", err)
			continue
		}
		for _, cap := range pack.Capabilities() {
			_ = capReg.Register(cap)
		}
		for _, factory := range pack.EnvironmentFactories() {
			_ = envReg.Register(factory)
		}
		allTemplates = append(allTemplates, pack.ObjectiveTemplates()...)
	}

	apiApp := api.NewApp(cfg, store, providers, toolReg, exporters, wt, hub, otel, capReg, envReg, domReg, allTemplates)
	return &Bootstrap{Config: cfg, App: apiApp, Store: store, Worktrees: wt}, nil
}

func ConfigPath() string {
	if p := os.Getenv("KARAKURI_CONFIG"); p != "" {
		return p
	}
	return "config/default.yaml"
}
