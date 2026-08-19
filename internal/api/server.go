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
	"github.com/bsenel/karakuri/internal/feature/container"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/feature/memory"
	"github.com/bsenel/karakuri/internal/feature/objective"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	"github.com/bsenel/karakuri/internal/feature/research"
	"github.com/bsenel/karakuri/internal/feature/twin"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	karakuriweb "github.com/bsenel/karakuri/web"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type App struct {
	Router *chi.Mux
	Loop   featureloop.Service // exposed so bootstrap can call ResumeStoredLoops post-construction
	Memory *memory.Service     // exposed so bootstrap can drive the retention scheduler
	// Reconcile is exposed so bootstrap can start the supervisor, for the
	// same reason Loop and Memory are: it owns a goroutine whose lifetime is
	// the process's, not a request's.
	Reconcile *featurereconcile.Service
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

	// Federation is the configured identity provider, or a zero value when
	// none is. It is never nil.
	Federation *karakuriauth.Federation
}

// Resolver authenticates a request from its access token: the Authorization
// header for API clients, or the access cookie for browsers.
//
// There is deliberately no query-parameter fallback. EventSource cannot set a
// header, which is the usual reason to allow one, but it does send cookies —
// so the cookie covers SSE too, without writing a credential into URLs that
// end up in access logs, proxy logs and Referer headers.
func (d AuthDeps) Resolver() auth.TokenResolver {
	local := auth.NewJWTResolver(d.Tokens, karakuriauth.AccessCookieName)
	external := d.Federation.Resolver()
	if external == nil {
		return local
	}
	// Local first: it is a signature check against a key already in memory,
	// while the federated one may reach for a key set over the network. The
	// chain continues past a failed verification precisely so a
	// provider-issued token — which is also a bearer token, and which the
	// local resolver will reject — still reaches the resolver that can verify
	// it. See auth.ChainResolver.
	return auth.ChainResolver{local, external}
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
	quotaDeps karakuriquota.Deps,
) *App {
	var memSvc *memory.Service
	if semanticBackend != nil {
		memSvc = memory.NewServiceWithSemantic(store, cfg.Memory.SemanticTopK, semanticBackend)
	} else {
		memSvc = memory.NewService(store, cfg.Memory.SemanticTopK)
	}
	twinSvc := twin.NewService(store, hub)
	containerSvc := container.NewService(store)
	// Objectives inherit their twin's containers, so a grant on an organisation
	// reaches the work being done in it and not only the twins themselves.
	objSvc := objective.NewService(store).WithContainers(containerSvc)
	for _, t := range templates {
		objSvc.RegisterTemplate(t)
	}
	cpSvc := checkpoint.NewService(store, hub)
	artSvc := artifact.NewService(store)
	resSvc := research.NewService(toolReg, artSvc)
	agentFactory := platformagent.NewFactory(providers, hub, otel)
	loopSvc := featureloop.NewService(store, agentFactory, capReg, envReg, memSvc, cpSvc, artSvc, wt, hub, otel, domReg, quotaDeps)
	// Closes the cycle: the loop raises checkpoints, and resolving one has to
	// reach back into the loop that is blocked on it. Constructor injection
	// cannot express that in either direction, so the second edge is wired
	// here, immediately after both services exist.
	cpSvc.SetResumer(loopSvc)
	reconcileSvc := featurereconcile.NewService(store, loopSvc, envReg, domReg, cpSvc, hub, featurereconcile.Config{
		Tick:               cfg.Reconcile.TickDuration(),
		MaxConcurrent:      cfg.Reconcile.MaxConcurrent,
		LeaseTTL:           cfg.Reconcile.LeaseTTLDuration(),
		BreakerFailures:    cfg.Reconcile.BreakerFailures,
		StallReconciles:    cfg.Reconcile.StallReconciles,
		DefaultMinInterval: cfg.Reconcile.DefaultMinIntervalDuration(),
		MaxBackoff:         cfg.Reconcile.MaxBackoffDuration(),
	})

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(middleware.SecurityHeaders)
	// 1 MiB request-body ceiling. Every handler decodes small JSON documents;
	// nothing legitimately posts more than this. See SECURITY_AUDIT.md F-05.
	r.Use(middleware.MaxBytes(1 << 20))
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
	// scoped attaches the containers a resource belongs to, so a binding on an
	// org or a team covers what is inside it. A resource in no container gets
	// no labels and decides exactly as it did before containers existed, which
	// is why every route can carry this without a flag.
	scoped := func(inner auth.ResourceFunc) auth.ResourceFunc {
		return karakuriauth.Scoped(containerSvc, inner)
	}
	// Twin routes also resolve the twin's owner so owner_equals conditions can
	// fire; one read per request, confined to routes that actually name a twin.
	ownedTwin := scoped(karakuriauth.OwnedTwinResource(store))
	twinRes := scoped(karakuriauth.TwinResource)
	objectiveRes := scoped(karakuriauth.ObjectiveResource)
	// List routes are the one place a collection ref carries the caller's own
	// containers, so a team-scoped binding can reach GET /twins at all. Which
	// rows come back is then the handler's filter, not this check.
	twinList := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionTwinRead, karakuriauth.TwinResource)
	objectiveList := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionObjectiveRead, karakuriauth.ObjectiveResource)
	// Routes whose real subject arrives in the body: the gate here answers "do
	// you hold this permission anywhere", and the handler re-checks against the
	// container or scope the request actually names. Without this a principal
	// bound to one organisation could not create a team inside it, because the
	// collection ref is "container:*" and no container-scoped binding matches
	// that.
	containerWrite := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionContainerWrite, karakuriauth.CollectionResource("container"))
	bindingWrite := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionAuthWrite, karakuriauth.CollectionResource("auth"))
	// Quota self-service and the spend report are the same shape: the subject
	// is a stored request or a set of containers, neither of which a URL-shaped
	// check can see, so the gate carries the caller's own containers and the
	// handler decides the rest.
	quotaAsk := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionQuotaRequest, karakuriauth.CollectionResource("quota"))
	quotaList := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionQuotaRead, karakuriauth.CollectionResource("quota"))
	quotaDecide := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionQuotaApprove, karakuriauth.CollectionResource("quota"))
	costRead := karakuriauth.ScopedCollection(authDeps.Authorizer,
		karakuriauth.ActionCostRead, karakuriauth.CollectionResource("cost"))
	containerRes := karakuriauth.ContainerResource(containerSvc)

	healthH := &handler.HealthHandler{Providers: providers, Tools: toolReg, Exporters: exporters, Worktrees: wt, RepoPath: cfg.Git.RepoPath}
	twinH := &handler.TwinHandler{Twins: twinSvc, Scopes: authDeps.Authorizer}
	objH := &handler.ObjectiveHandler{Objectives: objSvc, Scopes: authDeps.Authorizer}
	loopH := &handler.LoopHandler{Loop: loopSvc, Store: store, Reconcile: reconcileSvc}
	cpH := &handler.CheckpointHandler{Checkpoints: cpSvc}
	artH := &handler.ArtifactHandler{Artifacts: artSvc}
	memH := &handler.MemoryHandler{Memory: memSvc}
	domH := &handler.DomainHandler{Domains: domReg, Capabilities: capReg}
	resH := &handler.ResearchHandler{Research: resSvc}
	evtH := &handler.EventsHandler{
		Hub:        hub,
		Scopes:     authDeps.Authorizer,
		Containers: containerSvc,
	}
	audH := &handler.AuditHandler{Store: store}
	contH := &handler.ContainerHandler{Containers: containerSvc, Authorizer: authDeps.Authorizer}
	quotaH := &handler.QuotaHandler{
		Quota:      quotaDeps,
		Authorizer: authDeps.Authorizer,
		Scopes:     authDeps.Authorizer,
		Containers: containerSvc,
	}
	authH := &handler.AuthHandler{
		Store:      authDeps.Store,
		Tokens:     authDeps.Tokens,
		Authorizer: authDeps.Authorizer,
		Catalog:    authDeps.Catalog,
		Cookies:    authDeps.Cookies,
		Containers: containerSvc,
	}
	ssoH := &handler.SSOHandler{
		Federation:        authDeps.Federation,
		Tokens:            authDeps.Tokens,
		Cookies:           authDeps.Cookies,
		InsecureAllowHTTP: cfg.Auth.Cookies.InsecureAllowHTTP,
	}

	// The public /auth/* routes have no principal to key the quota limiter on,
	// so they get an IP-keyed brake against credential brute-force and spraying.
	// See SECURITY_AUDIT.md F-03. 10 attempts/min with a burst of 20 admits an
	// interactive user retrying a typo while denying an automated attacker.
	loginLimiter := middleware.NewIPRateLimiter(10, 20)

	r.Route("/api/v1", func(r chi.Router) {
		// Public: a load balancer and the SPA's login screen both need to
		// reach a server they cannot yet authenticate against.
		r.Get("/health", healthH.ServeHTTP)

		// Public credential-bearing routes: the credential *is* the request, so
		// requiring a credential to reach them would be circular. They are
		// rate-limited by client IP because there is no principal to key on.
		r.Group(func(r chi.Router) {
			r.Use(loginLimiter.Middleware)
			r.Post("/auth/token", authH.Token)
			r.Post("/auth/refresh", authH.Refresh)

			// Federated login, for the same reason. These are mounted whatever
			// the configured provider is: with none, they answer 404 rather
			// than vanishing, so a misconfigured client gets an explanation
			// instead of a route that silently does not exist.
			r.Get("/auth/sso/config", ssoH.Config)
			r.Method(http.MethodGet, "/auth/sso/login", ssoH.Login())
			r.Method(http.MethodGet, "/auth/sso/callback", ssoH.Callback())
			r.Post("/auth/sso/exchange", ssoH.Exchange)
			r.Method(http.MethodGet, "/auth/saml/metadata", ssoH.Metadata())
			r.Method(http.MethodPost, "/auth/saml/acs", ssoH.ACS())
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.Authenticate(authDeps.Resolver()))
			// After Authenticate, because the limiter keys on the principal;
			// before the permission gates, because refusing on rate is cheaper
			// than resolving a policy — and a caller who is over their limit
			// should get the same answer whether or not they would have been
			// allowed through.
			r.Use(quotaDeps.Limiter())

			// Available to any authenticated principal: reading your own
			// identity and ending your own session need no permission.
			r.Get("/auth/me", authH.Me)
			r.Post("/auth/revoke", authH.Revoke)

			r.Route("/auth", func(r chi.Router) {
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/users", authH.ListUsers)
				r.With(require(karakuriauth.ActionAuthWrite, nil)).Post("/users", authH.CreateUser)
				r.With(require(karakuriauth.ActionAuthWrite, nil)).Delete("/users/{id}", authH.DeleteUser)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/bindings", authH.ListBindings)
				r.With(require(karakuriauth.ActionAuthWrite, bindingWrite)).Post("/bindings", authH.CreateBinding)
				// Revoking is bounded by the same rule as granting, re-checked
				// in the handler against the scope the binding actually names.
				r.With(require(karakuriauth.ActionAuthWrite, bindingWrite)).Delete("/bindings/{id}", authH.DeleteBinding)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/roles", authH.ListRoles)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/policies", authH.ListPolicies)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Get("/catalog", authH.ListCatalog)
				r.With(require(karakuriauth.ActionAuthRead, nil)).Post("/check", authH.Check)
			})

			r.Route("/twins", func(r chi.Router) {
				r.With(require(karakuriauth.ActionTwinCreate, nil)).Post("/", twinH.Create)
				r.With(require(karakuriauth.ActionTwinRead, twinList)).Get("/", twinH.List)
				r.With(require(karakuriauth.ActionTwinRead, ownedTwin)).Get("/{id}", twinH.Get)
				r.With(require(karakuriauth.ActionTwinUpdate, ownedTwin)).Put("/{id}", twinH.Update)
				r.With(require(karakuriauth.ActionTwinBind, ownedTwin)).Put("/{id}/bindings", twinH.SetBindings)
				r.With(require(karakuriauth.ActionTwinRead, twinRes)).Get("/{id}/events", evtH.StreamTwin)
			})

			r.Route("/objectives", func(r chi.Router) {
				r.With(require(karakuriauth.ActionObjectiveCreate, nil)).Post("/", objH.Create)
				r.With(require(karakuriauth.ActionObjectiveRead, objectiveList)).Get("/", objH.List)
				r.With(require(karakuriauth.ActionObjectiveRead, nil)).Get("/templates", objH.ListTemplates)
				r.With(require(karakuriauth.ActionObjectiveRead, objectiveRes)).Get("/{id}", objH.Get)
				r.With(require(karakuriauth.ActionObjectiveUpdate, objectiveRes)).Post("/{id}/status", objH.UpdateStatus)
				r.With(require(karakuriauth.ActionObjectiveRead, objectiveRes)).Get("/{id}/events", evtH.StreamObjective)
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

			r.Route("/quota", func(r chi.Router) {
				r.With(require(karakuriauth.ActionQuotaRead, nil)).Get("/", quotaH.Config)
				r.With(require(karakuriauth.ActionQuotaRead, nil)).Get("/usage", quotaH.Usage)
				// Resetting somebody's counters is an operator override, not an
				// ordinary operation.
				r.With(require(karakuriauth.ActionQuotaAdmin, nil)).Post("/reset", quotaH.Reset)
				// Self-service (Phase 18). The gate answers "may you ask",
				// "may you decide", "may you list"; *which* subject and which
				// rows are the handler's, because the subject arrives inside a
				// stored request rather than in the URL. All three carry the
				// caller's own containers for the same reason container writes
				// do: the collection ref is "quota:*", which no binding scoped
				// to an organisation matches.
				r.With(require(karakuriauth.ActionQuotaRequest, quotaAsk)).Post("/requests", quotaH.SubmitRequest)
				r.With(require(karakuriauth.ActionQuotaRead, quotaList)).Get("/requests", quotaH.ListRequests)
				r.With(require(karakuriauth.ActionQuotaApprove, quotaDecide)).Post("/requests/{id}/decide", quotaH.Decide)
				// The limits themselves (Phase 19). Reading is quota:read like
				// the rest; writing is quota:admin and deliberately unscoped —
				// approving a raise for a twin you administer is a tenant
				// decision, and moving the ceiling for everybody is not.
				// The raises in force, filtered to the subjects the caller
				// could have approved for.
				r.With(require(karakuriauth.ActionQuotaRead, quotaList)).Get("/overrides", quotaH.Overrides)
				r.With(require(karakuriauth.ActionQuotaApprove, quotaDecide)).Delete("/overrides/{subject}/{name}", quotaH.RevokeOverride)
				r.With(require(karakuriauth.ActionQuotaRead, quotaList)).Get("/tiers", quotaH.Tiers)
				r.With(require(karakuriauth.ActionQuotaAdmin, nil)).Put("/tiers/{name}", quotaH.SetTier)
				r.With(require(karakuriauth.ActionQuotaAdmin, nil)).Delete("/tiers/{name}", quotaH.ResetTier)
			})

			// The tenancy tree. Every mutation is re-checked in the handler
			// against the container it actually names, because the parent
			// arrives in the body rather than the URL.
			r.Route("/containers", func(r chi.Router) {
				r.With(require(karakuriauth.ActionContainerWrite, containerWrite)).Post("/", contH.Create)
				r.With(require(karakuriauth.ActionContainerRead, nil)).Get("/", contH.List)
				r.With(require(karakuriauth.ActionContainerRead, nil)).Get("/resources", contH.GetResourceContainers)
				r.With(require(karakuriauth.ActionContainerWrite, containerWrite)).Post("/resources", contH.SetResourceContainers)
				r.With(require(karakuriauth.ActionContainerRead, containerRes)).Get("/{id}", contH.Get)
				r.With(require(karakuriauth.ActionContainerWrite, containerRes)).Post("/{id}/name", contH.Rename)
				r.With(require(karakuriauth.ActionContainerMove, containerRes)).Post("/{id}/parent", contH.Reparent)
				r.With(require(karakuriauth.ActionContainerWrite, containerRes)).Delete("/{id}", contH.Delete)
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
			// The deployment-wide stream a dashboard follows. The route gate
			// answers "may you watch anything"; which events reach this
			// subscriber is decided per event, because the key names
			// everything and no URL-shaped check can narrow that.
			r.With(require(karakuriauth.StreamAction, twinList)).Get("/events", evtH.StreamAll)

			r.With(require(karakuriauth.ActionAuditRead, nil)).Get("/audit", audH.List)
			r.With(require(karakuriauth.ActionAuditRead, nil)).Get("/audit/{id}", audH.Get)
			// Filtered to the containers the caller may see, from the same
			// bindings the twin listing reads — a report must not be a way
			// around the tenancy those enforce.
			r.With(require(karakuriauth.ActionCostRead, costRead)).Get("/cost", quotaH.CostReport)
		})
	})

	// Mount the embedded React SPA at the root, AFTER /api/v1 has been
	// registered so REST + SSE win over the catch-all SPA fallback.
	r.Handle("/*", karakuriweb.Handler())

	return &App{Router: r, Loop: loopSvc, Memory: memSvc, Reconcile: reconcileSvc}
}

func (a *App) Handler() http.Handler { return a.Router }
