package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	domainagri "github.com/bsenel/karakuri/domains/agriculture"
	domainconsult "github.com/bsenel/karakuri/domains/consulting"
	domainhc "github.com/bsenel/karakuri/domains/healthcare"
	domainkrk "github.com/bsenel/karakuri/domains/karakuri"
	domainlegal "github.com/bsenel/karakuri/domains/legal"
	domainmech "github.com/bsenel/karakuri/domains/mechanical"
	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/api"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	corememory "github.com/bsenel/karakuri/internal/core/memory"
	objectivepkg "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/container"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	"github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	platmem "github.com/bsenel/karakuri/internal/platform/memory"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	plattelemetry "github.com/bsenel/karakuri/internal/platform/telemetry"
	"github.com/bsenel/karakuri/internal/platform/tools"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"gorm.io/gorm"
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
		domainkrk.NewWithTools(toolReg),
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

	authDeps, err := BuildAuth(ctx, gormDB, store, cfg)
	if err != nil {
		return nil, err
	}

	quotaDeps, err := BuildQuota(ctx, gormDB, cfg, hub)
	if err != nil {
		return nil, err
	}

	// The read-only port through which Karakuri observes itself (Phase 22).
	// Wired on the registry so every environment factory receives it on its
	// BuildContext; every pack but karakuri's ignores it.
	envReg.SetTelemetry(plattelemetry.New(store, quotaDeps))

	apiApp := api.NewApp(cfg, store, providers, toolReg, exporters, wt, hub, otel, capReg, envReg, domReg, allTemplates, semanticBackend, promHandler, authDeps, quotaDeps)

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

	// Cost retention (Phase 18). One event per model call and per tool call
	// adds up, so raw rows age out while the daily rollup does not — a shorter
	// horizon costs the drill-down and not the totals.
	startCostRetention(ctx, quotaDeps, cfg.Quota.CostRetentionDays)

	// The supervisor that holds standing objectives at their declared state
	// (Phase 20). It does nothing on a deployment that has declared none, so
	// it starts by default; the config flag is the kill switch for a
	// deployment that wants the feature off outright.
	if cfg.Reconcile.IsEnabled() {
		apiApp.Reconcile.Start(ctx)
		// The digest sender, which reports on what the supervisor did. It logs
		// its own refusal when disabled, which it is by default.
		apiApp.Reports.Start(ctx)
	} else {
		slog.Info("reconcile supervisor disabled by configuration",
			"note", "standing objectives keep their state and stop becoming due")
	}

	return &Bootstrap{Config: cfg, App: apiApp, Store: store, Worktrees: wt}, nil
}

// startCostRetention prunes raw cost events on a daily tick.
//
// Daily rather than hourly because the horizon is measured in days: sweeping
// twelve times between two cutoffs deletes nothing eleven of those times. The
// first sweep runs a minute after boot rather than a day later, so a deployment
// that has just lowered its retention sees the effect without waiting a day,
// and a restart loop cannot postpone pruning indefinitely.
//
// Zero retention keeps everything, which is why this is not gated on an
// Enabled flag: the horizon itself says whether to sweep.
func startCostRetention(ctx context.Context, deps karakuriquota.Deps, days int) {
	if days <= 0 {
		return
	}
	retention := time.Duration(days) * 24 * time.Hour
	slog.Info("cost event retention enabled",
		"days", days,
		"note", "the daily rollup is kept; only per-event rows are pruned")

	go func() {
		sweep := func() {
			n, err := deps.SweepCosts(ctx, retention, time.Now().UTC())
			if err != nil {
				slog.Warn("cost retention sweep failed", "err", err)
				return
			}
			if n > 0 {
				slog.Info("cost events pruned", "rows", n, "older_than_days", days)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
			sweep()
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()
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

// BuildAuth assembles the authentication and authorization stack: the store on
// Karakuri's own database connection, the JWT keyring, the token service, and
// the enforcer that gates every /api/v1 route.
//
// Exported so the integration suite exercises this exact wiring rather than a
// parallel copy that could drift from what the server actually runs.
//
// Every failure here is fatal. There is no degraded mode — a Karakuri that
// cannot authorize requests must not serve them, and the most likely failure
// (no signing key configured) is precisely the one where continuing would mean
// running with predictable tokens.
func BuildAuth(ctx context.Context, gormDB *gorm.DB, store storage.StorageAdapter, cfg *config.Config) (api.AuthDeps, error) {
	authStore, err := karakuriauth.NewStore(gormDB)
	if err != nil {
		return api.AuthDeps{}, err
	}
	if err := authStore.Migrate(ctx); err != nil {
		return api.AuthDeps{}, fmt.Errorf("auth schema: %w", err)
	}

	keyring, err := karakuriauth.NewKeyring(cfg.Auth.JWT)
	if err != nil {
		return api.AuthDeps{}, err
	}

	tokens, err := auth.NewTokenService(authStore, authStore, keyring, auth.TokenConfig{
		Issuer:     cfg.Auth.JWT.Issuer,
		Audience:   cfg.Auth.JWT.Audience,
		AccessTTL:  cfg.Auth.JWT.AccessTTLDuration(),
		RefreshTTL: cfg.Auth.JWT.RefreshTTLDuration(),
	})
	if err != nil {
		return api.AuthDeps{}, fmt.Errorf("token service: %w", err)
	}

	catalog := karakuriauth.NewCatalog()
	if err := karakuriauth.Seed(ctx, authStore, tokens, catalog, cfg.Auth.Bootstrap); err != nil {
		return api.AuthDeps{}, fmt.Errorf("seed auth model: %w", err)
	}

	// The container service resolves the org, team and project names in the
	// role map to IDs. It reads Karakuri's own tables, so it is built here
	// rather than passed in — nothing else in this function needs it.
	federation, err := karakuriauth.BuildFederation(ctx, cfg, authStore, container.NewService(store))
	if err != nil {
		return api.AuthDeps{}, err
	}
	if federation.Enabled() {
		slog.Info("federated identity enabled", "provider", federation.Kind)
	}

	authorizer := auth.NewAuthorizer(authStore)
	enforcer := auth.NewEnforcer(authorizer)
	// Every denial lands in the same audit log as authority-bounds escalations,
	// so `krk audit --kind authz_denied` shows attempts alongside approvals.
	enforcer.OnDeny = karakuriauth.AuditDenial(store)
	enforcer.OnError = func(r *http.Request, err error) {
		slog.Error("authorization could not be evaluated",
			"path", karakuriauth.SanitizeLogValue(r.URL.Path), "err", err)
	}

	return api.AuthDeps{
		Store:      authStore,
		Tokens:     tokens,
		Authorizer: authorizer,
		Catalog:    catalog,
		Enforcer:   enforcer,
		Cookies:    karakuriauth.CookieConfig(cfg.Auth),
		Federation: federation,
	}, nil
}

// BuildQuota constructs the rate limiter and quota tiers from configuration.
//
// Exported alongside BuildAuth and for the same reason: the integration suite
// exercises this exact wiring rather than a parallel copy that could drift.
//
// Failures here are fatal. A limiter that cannot be built is one that would
// admit everything, and a deployment that asked for a limit and silently did
// not get one is worse off than one that never configured it — it believes it
// is protected.
func BuildQuota(ctx context.Context, gormDB *gorm.DB, cfg *config.Config, hub *event.Hub) (karakuriquota.Deps, error) {
	// The quota module shares the application's pool rather than opening its
	// own. The counters only need it on the SQL backend, but the overrides,
	// requests and cost ledger need it whichever backend is counting — an
	// approval and a spend report have to survive a restart on every
	// deployment, not only the ones that chose SQL for their rate limiter.
	var sqlDB *sql.DB
	if gormDB != nil {
		db, err := gormDB.DB()
		if err != nil {
			return karakuriquota.Deps{}, fmt.Errorf("quota: %w", err)
		}
		sqlDB = db
	}
	return karakuriquota.Build(ctx, cfg.Quota, sqlDB, hub)
}
