package api

import (
	"net/http"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/api/middleware"
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
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type App struct {
	Router *chi.Mux
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
	r.Use(middleware.BearerAuth(cfg.Auth.Token))

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

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthH.ServeHTTP)

		r.Route("/twins", func(r chi.Router) {
			r.Post("/", twinH.Create)
			r.Get("/", twinH.List)
			r.Get("/{id}", twinH.Get)
			r.Put("/{id}", twinH.Update)
			r.Put("/{id}/bindings", twinH.SetBindings)
			r.Get("/{id}/events", evtH.StreamTwin)
		})

		r.Route("/objectives", func(r chi.Router) {
			r.Post("/", objH.Create)
			r.Get("/", objH.List)
			r.Get("/templates", objH.ListTemplates)
			r.Get("/{id}", objH.Get)
			r.Post("/{id}/status", objH.UpdateStatus)
			r.Get("/{id}/events", evtH.StreamObjective)
		})

		r.Route("/loops", func(r chi.Router) {
			r.Post("/", loopH.Start)
			r.Get("/{id}/status", loopH.Status)
			r.Post("/{id}/resume", loopH.Resume)
		})

		r.Route("/checkpoints", func(r chi.Router) {
			r.Get("/", cpH.ListPending)
			r.Get("/{id}", cpH.Get)
			r.Post("/{id}/resolve", cpH.Resolve)
		})

		r.Route("/artifacts", func(r chi.Router) {
			r.Get("/", artH.List)
			r.Post("/", artH.Write)
			r.Get("/{sha}", artH.Get)
			r.Get("/{sha}/diff/{other}", artH.Diff)
		})

		r.Route("/memory", func(r chi.Router) {
			r.Post("/store", memH.Store)
			r.Post("/recall", memH.Recall)
			r.Post("/forget", memH.Forget)
		})

		r.Route("/domains", func(r chi.Router) {
			r.Get("/", domH.List)
			r.Get("/capabilities", domH.ListCapabilities)
			r.Get("/{id}/conformance", domH.Conformance)
		})

		r.Post("/research", resH.Run)
	})

	return &App{Router: r}
}

func (a *App) Handler() http.Handler { return a.Router }
