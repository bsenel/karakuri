package api

import (
	"net/http"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/api/middleware"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	corememory "github.com/bsenel/karakuri/internal/core/memory"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/feature/memory"
	"github.com/bsenel/karakuri/internal/feature/objective"
	"github.com/bsenel/karakuri/internal/feature/research"
	"github.com/bsenel/karakuri/internal/feature/twin"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	karakuriweb "github.com/bsenel/karakuri/web"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type App struct {
	Router *chi.Mux
	Loop   featureloop.Service // exposed so bootstrap can call ResumeStoredLoops post-construction
	Memory *memory.Service     // exposed so bootstrap can drive the retention scheduler
}

// AuthDeps carries the authentication and authorization wiring built by
// internal/app.Bootstrap. It is a struct rather than four more positional
// parameters because they always travel together.
type AuthDeps struct {
	Store      auth.Store
	Tokens     *auth.TokenService
	Authorizer *auth.StoreAuthorizer
	Catalog    *auth.Catalog
	Enforcer   *auth.Enforcer
	Cookies    auth.CookieConfig
}

// Resolver authenticates a request from its access token: the Authorization
// header for API clients, or the access cookie for browsers.
//
// There is deliberately no query-parameter fallback. EventSource cannot set a
// header, which is the usual reason to allow one, but it does send cookies —
// so the cookie covers SSE too, without writing a credential into URLs that
// end up in access logs, proxy logs and Referer headers.
func (d AuthDeps) Resolver() auth.TokenResolver {
	return auth.NewJWTResolver(d.Tokens, karakuriauth.AccessCookieName)
}

func NewApp(
	cfg *config.Config,
	store storage.StorageAdapter,
	providers *llm.Registry,
	toolReg *tools.Registry,
	exporters *observability.ExporterRegistry,
	wt git.WorktreeManager,
	hub *event.Hub,
	otel *observability.OTel,
	capReg *capability.Registry,
	envReg *environment.Registry,
	domReg *domain.Registry,
	templates []coreobjective.Template,
	semanticBackend corememory.Memory, // optional override; nil → default SQLite keyword
	prometheusHandler http.Handler, // optional; mounted at /metrics outside auth when non-nil
	authDeps AuthDeps,
) *App {
	var memSvc *memory.Service
	if semanticBackend != nil {
		memSvc = memory.NewServiceWithSemantic(store, cfg.Memory.SemanticTopK, semanticBackend)
	} else {
		memSvc = memory.NewService(store, cfg.Memory.SemanticTopK)
	}
	twinSvc := twin.NewService(store, hub)
	objSvc := objective.NewService(store)
	for _, t := range templates {
		objSvc.RegisterTemplate(t)
	}
	cpSvc := checkpoint.NewService(store, hub)
	artSvc := artifact.NewService(store)
	resSvc := research.NewService(toolReg, artSvc)
	agentFactory := platformagent.NewFactory(providers, hub, otel)
	loopSvc := featureloop.NewService(store, agentFactory, capReg, envReg, memSvc, cpSvc, artSvc, wt, hub, otel, domReg)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	// Authentication is scoped to /api/v1 only — the SPA itself (HTML + assets)
	// is public so unauthenticated visitors can reach the login screen.

	// Prometheus scrape endpoint at /metrics, mounted at the router root
	// outside the authenticated scope. Prometheus scrapers don't authenticate,
	// so /metrics has to be publicly readable when the exporter is enabled.
	if prometheusHandler != nil {
		r.Handle("/metrics", prometheusHandler)
	}

	// require is the per-route permission gate. Every /api/v1 route below is
	// wrapped in one; the readable form of this mapping — and what the
	// integration suite walks to assert 200/403 per role — is
	// internal/auth.Routes().
	require := func(action auth.Action, resource auth.ResourceFunc) func(http.Handler) http.Handler {
		return authDeps.Enforcer.Require(action, resource)
	}
	// Twin routes resolve the twin's owner so owner_equals conditions can fire;
	// one read per request, confined to routes that actually name a twin.
	ownedTwin := karakuriauth.OwnedTwinResource(store)

	healthH := &handler.HealthHandler{Providers: providers, Tools: toolReg, Exporters: exporters, Worktrees: wt, RepoPath: cfg.Git.RepoPath}
	twinH := &handler.TwinHandler{Twins: twinSvc}
	objH := &handler.ObjectiveHandler{Objectives: objSvc}
	loopH := &handler.LoopHandler{Loop: loopSvc}
	cpH := &handler.CheckpointHandler{Checkpoints: cpSvc}
	artH := &handler.ArtifactHandler{Artifacts: artSvc}
	memH := &handler.MemoryHandler{Memory: memSvc}
	domH := &handler.DomainHandler{Domains: domReg, Capabilities: capReg}
	resH := &handler.ResearchHandler{Research: resSvc}
	evtH := &handler.EventsHandler{Hub: hub}
	audH := &handler.AuditHandler{Store: store}
	authH := &handler.AuthHandler{
		Store:      authDeps.Store,
		Tokens:     authDeps.Tokens,
		Authorizer: authDeps.Authorizer,
		Catalog:    authDeps.Catalog,
		Cookies:    authDeps.Cookies,
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public: a load balancer and the SPA's login screen both need to
		// reach a server they cannot yet authenticate against.
		r.Get("/health", healthH.ServeHTTP)

		// Public: for these three the credential *is* the request body, so
		// requiring a credential to reach them would be circular.
		r.Post("/auth/token", authH.Token)
		r.Post("/auth/refresh", authH.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(auth.Authenticate(authDeps.Resolver()))

			// Available to any authenticated principal: reading your own
			// identity and ending your own session need no permission.
			r.Get("/auth/me", authH.Me)
			r.Post("/auth/revoke", authH.Revoke)

			r.Route("/auth", func(r chi.Router) {
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/users", authH.ListUsers)
				r.With(require(karakuriauth.ActionAuthWrite, nil)).Post("/users", authH.CreateUser)
				r.With(require(karakuriauth.ActionAuthWrite, nil)).Post("/bindings", authH.CreateBinding)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/roles", authH.ListRoles)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/policies", authH.ListPolicies)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/catalog", authH.ListCatalog)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Post("/check", authH.Check)
			})

			r.Route("/twins", func(r chi.Router) {
				r.With(require(karakuriauth.ActionTwinCreate, nil)).Post("/", twinH.Create)
				r.With(require(karakuriauth.ActionTwinRead, nil)).Get("/", twinH.List)
				r.With(require(karakuriauth.ActionTwinRead, ownedTwin)).Get("/{id}", twinH.Get)
				r.With(require(karakuriauth.ActionTwinUpdate, ownedTwin)).Put("/{id}", twinH.Update)
				r.With(require(karakuriauth.ActionTwinBind, ownedTwin)).Put("/{id}/bindings", twinH.SetBindings)
				r.With(require(karakuriauth.ActionTwinRead, karakuriauth.TwinResource)).Get("/{id}/events", evtH.StreamTwin)
			})

			r.Route("/objectives", func(r chi.Router) {
				r.With(require(karakuriauth.ActionObjectiveCreate, nil)).Post("/", objH.Create)
				r.With(require(karakuriauth.ActionObjectiveRead, nil)).Get("/", objH.List)
				r.With(require(karakuriauth.ActionObjectiveRead, nil)).Get("/templates", objH.ListTemplates)
				r.With(require(karakuriauth.ActionObjectiveRead, karakuriauth.ObjectiveResource)).Get("/{id}", objH.Get)
				r.With(require(karakuriauth.ActionObjectiveUpdate, karakuriauth.ObjectiveResource)).Post("/{id}/status", objH.UpdateStatus)
				r.With(require(karakuriauth.ActionObjectiveRead, karakuriauth.ObjectiveResource)).Get("/{id}/events", evtH.StreamObjective)
			})

			r.Route("/loops", func(r chi.Router) {
				r.With(require(karakuriauth.ActionLoopStart, nil)).Post("/", loopH.Start)
				r.With(require(karakuriauth.ActionLoopRead, karakuriauth.LoopResource)).Get("/{id}/status", loopH.Status)
				r.With(require(karakuriauth.ActionLoopResume, karakuriauth.LoopResource)).Post("/{id}/resume", loopH.Resume)
			})

			r.Route("/checkpoints", func(r chi.Router) {
				r.With(require(karakuriauth.ActionCheckpointRead, nil)).Get("/", cpH.ListPending)
				r.With(require(karakuriauth.ActionCheckpointRead, karakuriauth.CheckpointResource)).Get("/{id}", cpH.Get)
				r.With(require(karakuriauth.ActionCheckpointResolve, karakuriauth.CheckpointResource)).Post("/{id}/resolve", cpH.Resolve)
			})

			r.Route("/artifacts", func(r chi.Router) {
				r.With(require(karakuriauth.ActionArtifactRead, nil)).Get("/", artH.List)
				r.With(require(karakuriauth.ActionArtifactWrite, nil)).Post("/", artH.Write)
				r.With(require(karakuriauth.ActionArtifactRead, karakuriauth.ArtifactResource)).Get("/{sha}", artH.Get)
				r.With(require(karakuriauth.ActionArtifactRead, karakuriauth.ArtifactResource)).Get("/{sha}/diff/{other}", artH.Diff)
			})

			r.Route("/memory", func(r chi.Router) {
				r.With(require(karakuriauth.ActionMemoryWrite, nil)).Post("/store", memH.Store)
				r.With(require(karakuriauth.ActionMemoryRead, nil)).Post("/recall", memH.Recall)
				r.With(require(karakuriauth.ActionMemoryForget, nil)).Post("/forget", memH.Forget)
			})

			r.Route("/domains", func(r chi.Router) {
				r.With(require(karakuriauth.ActionDomainRead, nil)).Get("/", domH.List)
				r.With(require(karakuriauth.ActionDomainRead, nil)).Get("/capabilities", domH.ListCapabilities)
				r.With(require(karakuriauth.ActionDomainRead, karakuriauth.DomainResource)).Get("/{id}/conformance", domH.Conformance)
			})

			r.With(require(karakuriauth.ActionResearchRun, nil)).Post("/research", resH.Run)

			// Authority-bounds audit log (Phase 13). Supports objective_id,
			// agent_id, kind, bounds_violation, since (RFC3339), and limit
			// query params. Auditor and admin only — a viewer can see the
			// system's work but not the record of who approved what.
			r.With(require(karakuriauth.ActionAuditRead, nil)).Get("/audit", audH.List)
		})
	})

	// Mount the embedded React SPA at the root, AFTER /api/v1 has been
	// registered so REST + SSE win over the catch-all SPA fallback.
	r.Handle("/*", karakuriweb.Handler())

	return &App{Router: r, Loop: loopSvc, Memory: memSvc}
}

func (a *App) Handler() http.Handler { return a.Router }
