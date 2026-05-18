package api

import (
	"net/http"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/api/middleware"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/autonomous"
	"github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/feature/delivery"
	"github.com/bsenel/karakuri/internal/feature/discovery"
	"github.com/bsenel/karakuri/internal/feature/orchestrator"
	"github.com/bsenel/karakuri/internal/feature/research"
	"github.com/bsenel/karakuri/internal/feature/session"
	"github.com/bsenel/karakuri/internal/feature/strategy"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/executor"
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

func NewApp(cfg *config.Config, store storage.StorageAdapter, providers *llm.Registry, toolReg *tools.Registry, exporters *observability.ExporterRegistry, wt git.WorktreeManager, hub *event.Hub, otel *observability.OTel) *App {
	exec := executor.NewRegistry(cfg.Executor)
	factory := platformagent.NewFactory(providers, hub, otel)

	artSvc := artifact.NewService(store)
	sessSvc := session.NewService(store)
	stratSvc := strategy.NewService(factory, artSvc)
	discSvc := discovery.NewService(factory, artSvc)
	reviewer := delivery.NewReviewer(factory, artSvc, store, hub)
	delivSvc := delivery.NewService(factory, artSvc, wt, store, reviewer, hub, otel)
	cpSvc := checkpoint.NewService(store, hub)
	planner := orchestrator.NewPlanner(cfg.WorkflowsDir)
	sched := orchestrator.NewScheduler(exec)
	orchSvc := orchestrator.NewService(store, planner, sched, factory, stratSvc, discSvc, delivSvc, hub, otel, exec)
	autoSvc := autonomous.NewService(toolReg, artSvc, sessSvc, hub, cfg.WorkflowsDir)
	researchSvc := research.NewService(toolReg, artSvc, sessSvc, store)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(middleware.BearerAuth(cfg.Auth.Token))

	health := &handler.HealthHandler{Providers: providers, Tools: toolReg, Exporters: exporters, Worktrees: wt, RepoPath: cfg.Git.RepoPath}
	sessH := &handler.SessionHandler{Sessions: sessSvc, Orchestrator: orchSvc}
	artH := &handler.ArtifactHandler{Artifacts: artSvc}
	evtH := &handler.EventsHandler{Hub: hub}
	cpH := &handler.CheckpointHandler{Checkpoints: cpSvc, Orchestrator: orchSvc}
	revH := &handler.ReviewHandler{Store: store}
	wtH := &handler.WorktreeHandler{Delivery: delivSvc}
	autoH := &handler.AutonomousHandler{Autonomous: autoSvc, Sessions: sessSvc}
	promoH := &handler.PromoteHandler{Autonomous: autoSvc}
	resH := &handler.ResearchHandler{Research: researchSvc}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health.ServeHTTP)
		r.Post("/sessions", sessH.Create)
		r.Get("/sessions", sessH.List)
		r.Get("/sessions/{sha}", sessH.Get)
		r.Delete("/sessions/{sha}", sessH.Delete)
		r.Post("/sessions/{sha}/run", sessH.Run)
		r.Get("/sessions/{sha}/status", sessH.Status)
		r.Get("/sessions/{sha}/events", evtH.Stream)
		r.Get("/sessions/{sha}/artifacts", artH.ListBySession)
		r.Get("/sessions/{sha}/checkpoints", cpH.List)
		r.Post("/sessions/{sha}/checkpoints/{id}/resolve", cpH.Resolve)
		r.Get("/sessions/{sha}/reviews", revH.ListBySession)
		r.Get("/sessions/{sha}/worktrees", wtH.List)
		r.Post("/sessions/{sha}/promote", promoH.Promote)
		r.Get("/artifacts/{sha}", artH.Get)
		r.Get("/artifacts/{sha}/diff/{other-sha}", artH.Diff)
		r.Get("/reviews/{sha}", revH.Get)
		r.Post("/auto/run", autoH.Run)
		r.Get("/auto/status", autoH.Status)
		r.Post("/auto/pause", autoH.Pause)
		r.Post("/auto/resume", autoH.Resume)
		r.Post("/research", resH.Run)
	})

	return &App{Router: r}
}

func (a *App) Handler() http.Handler { return a.Router }
