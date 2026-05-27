package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api"
	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	corememory "github.com/bsenel/karakuri/internal/core/memory"
	objectivepkg "github.com/bsenel/karakuri/internal/core/objective"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	"github.com/bsenel/karakuri/internal/platform/db"
	platmem "github.com/bsenel/karakuri/internal/platform/memory"
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
	if claude.UsingCLIFallback() {
		slog.Info("llm provider using CLI fallback", "provider", "claude", "reason", "ANTHROPIC_API_KEY unset; routing through `claude` CLI")
	}
	gemini := llm.NewGeminiProvider()
	providers.Register(gemini)
	if gemini.UsingCLIFallback() {
		slog.Info("llm provider using CLI fallback", "provider", "gemini", "reason", "GOOGLE_API_KEY/GOOGLE_AI_API_KEY unset; routing through `gemini` CLI")
	}
	providers.Register(llm.NewCursorProvider())
	providers.Register(llm.NewCopilotProvider())

	exporters := observability.NewExporterRegistry()
	// Hoisted so api.NewApp can mount /metrics when prometheus is registered.
	var promExporter *observability.PrometheusExporter

	// registerRemote wraps a remote (network-backed) exporter in
	// RetryExporter so transient blips don't drop a batch.
	registerRemote := func(e observability.Exporter) {
		exporters.Register(observability.NewRetryExporter(e, observability.RetryConfig{}))
	}
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
			// Local file writes are synchronous to disk — no retry wrapper.
			exporters.Register(observability.NewLocalFileExporter(ec.Path, mfmt, lfmt).
				WithRotation(ec.Rotation.MaxSizeMB, ec.Rotation.MaxAgeDays))
		case "aws":
			aws := observability.NewAWSExporter()
			if aws.Active() {
				registerRemote(aws)
			} else {
				slog.Warn("aws exporter declared but inactive — AWS_REGION not set or credentials missing")
			}
		case "datadog":
			dd := observability.NewDatadogExporter()
			if dd.Active() {
				registerRemote(dd)
			} else {
				slog.Warn("datadog exporter declared but inactive — DD_API_KEY not set")
			}
		case "newrelic":
			nr := observability.NewNewRelicExporter()
			if nr.Active() {
				registerRemote(nr)
			} else {
				slog.Warn("newrelic exporter declared but inactive — NEW_RELIC_LICENSE_KEY not set")
			}
		case "elasticsearch":
			es := observability.NewElasticsearchExporter()
			if es.Active() {
				registerRemote(es)
			} else {
				slog.Warn("elasticsearch exporter declared but inactive — ELASTICSEARCH_URL not set")
			}
		case "loki":
			lk := observability.NewLokiExporter()
			if lk.Active() {
				registerRemote(lk)
			} else {
				slog.Warn("loki exporter declared but inactive — LOKI_URL not set")
			}
		case "otlp":
			ot := observability.NewOTLPExporter()
			if ot.Active() {
				registerRemote(ot)
			} else {
				slog.Warn("otlp exporter declared but inactive — OTEL_EXPORTER_OTLP_ENDPOINT not set")
			}
		case "prometheus":
			promExporter = observability.NewPrometheusExporter()
			// Prometheus is always active once registered (scrape mode has no
			// credential requirement). Push mode adds an outbound POST that
			// gets the retry wrapper benefits via the chain like any other
			// remote, so we register the raw exporter and skip the wrapper.
			exporters.Register(promExporter)
		}
	}
	otel := observability.NewOTel(exporters)

	wt, err := git.NewGoGitWorktreeManager(cfg.Git)
	if err != nil {
		return nil, err
	}

	toolReg := tools.NewRegistryFromConfig(cfg.Tools)
	hub := event.NewHub()

	capReg := capability.NewRegistry()
	envReg := environment.NewRegistry()
	domReg := domain.NewRegistry()

	ctx := context.Background()

	// Register universal capabilities
	for _, cap := range capability.Universal {
		_ = capReg.Register(cap)
	}

	// Register domain packs (software pack uses the tool registry; others are stubs)
	allPacks := []domain.Pack{
		domainsw.NewWithTools(toolReg),
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
	var activePacks []domain.Pack
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
		activePacks = append(activePacks, pack)
	}

	// Cross-pack capability/environment/agent collision audit (Phase 13).
	// Cross-domain objectives recruit from multiple packs in one loop; a
	// shared ID across packs would make agent/env resolution ambiguous.
	// Logged as WARN — operators may intentionally re-export an ID — but
	// surfaced loudly so it can't go unnoticed.
	if len(activePacks) >= 2 {
		for _, res := range conformance.CheckCrossPackCollisions(activePacks...) {
			if !res.Passed {
				slog.Warn("cross-pack conformance check failed", "check", res.Check, "msg", res.Message)
			}
		}
	}

	// Pick semantic backend per config. Only pgvector requires a non-default
	// constructor; the SQLite keyword fallback is the default path (nil here).
	var semanticBackend corememory.Memory
	if cfg.Memory.VectorBackend == "pgvector" {
		if cfg.Database.Driver != "postgres" {
			slog.Warn("memory.vector_backend=pgvector requires database.driver=postgres; falling back to SQLite keyword recall")
		} else {
			pgvec, err := platmem.NewSemanticMemoryPgVector(ctx, gormDB, cfg.Memory.EmbeddingDim)
			if err != nil {
				slog.Warn("pgvector backend init failed; falling back to SQLite keyword recall", "err", err)
			} else {
				semanticBackend = pgvec
				slog.Info("semantic memory backend: pgvector", "embedding_dim", cfg.Memory.EmbeddingDim)
			}
		}
	}

	var promHandler http.Handler
	if promExporter != nil {
		promHandler = promExporter
	}
	apiApp := api.NewApp(cfg, store, providers, toolReg, exporters, wt, hub, otel, capReg, envReg, domReg, allTemplates, semanticBackend, promHandler)

	// Resume any non-completed loops left behind by a previous server process
	// (Phase 11). Failures are logged but don't block startup — a working
	// REST API is more valuable than a clean replay on a corrupt state row.
	if err := apiApp.Loop.ResumeStoredLoops(ctx); err != nil {
		slog.Warn("loop resume failed at startup", "err", err)
	}

	// Memory retention scheduler (Phase 13). Off by default; enable in config
	// once the operator has measured growth and decided on per-tier TTLs.
	if cfg.Memory.Retention.Enabled {
		startRetentionLoop(ctx, apiApp.Memory, cfg.Memory.Retention)
	}

	return &Bootstrap{Config: cfg, App: apiApp, Store: store, Worktrees: wt}, nil
}

// startRetentionLoop launches a background goroutine that periodically calls
// MemoryService.RunRetention with the per-tier policies derived from config.
// The goroutine exits when ctx is cancelled (process shutdown).
func startRetentionLoop(ctx context.Context, memSvc *featurememory.Service, rc config.MemoryRetentionConfig) {
	interval := time.Duration(rc.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = time.Hour
	}
	slog.Info("memory retention scheduler enabled",
		"interval_minutes", int(interval.Minutes()),
		"working_ttl_minutes", rc.WorkingTTLMinutes,
		"episodic_ttl_days", rc.EpisodicTTLDays,
		"semantic_ttl_days", rc.SemanticTTLDays,
		"semantic_min_score", rc.SemanticMinScore,
	)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				set := buildRetentionSet(rc)
				if err := memSvc.RunRetention(ctx, set); err != nil {
					slog.Warn("memory retention sweep failed", "err", err)
				}
			}
		}
	}()
}

// buildRetentionSet translates the static config into a per-tier policy at
// the moment of each sweep. Building it fresh on every tick is intentional —
// the "before" cutoffs must advance with wall time, not stay frozen at boot.
func buildRetentionSet(rc config.MemoryRetentionConfig) featurememory.RetentionPolicySet {
	now := time.Now().UTC()
	var set featurememory.RetentionPolicySet
	if rc.WorkingTTLMinutes > 0 {
		before := now.Add(-time.Duration(rc.WorkingTTLMinutes) * time.Minute)
		set.Working.Before = &before
	}
	if rc.EpisodicTTLDays > 0 {
		before := now.AddDate(0, 0, -rc.EpisodicTTLDays)
		set.Episodic.Before = &before
	}
	if rc.SemanticTTLDays > 0 {
		before := now.AddDate(0, 0, -rc.SemanticTTLDays)
		set.Semantic.Before = &before
	}
	if rc.SemanticMinScore > 0 {
		set.Semantic.MinScore = rc.SemanticMinScore
	}
	return set
}

func ConfigPath() string {
	if p := os.Getenv("KARAKURI_CONFIG"); p != "" {
		return p
	}
	return "config/default.yaml"
}
