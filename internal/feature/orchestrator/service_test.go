package orchestrator_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/delivery"
	"github.com/bsenel/karakuri/internal/feature/discovery"
	"github.com/bsenel/karakuri/internal/feature/orchestrator"
	"github.com/bsenel/karakuri/internal/feature/session"
	"github.com/bsenel/karakuri/internal/feature/strategy"
	"github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/executor"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func TestStrategyRun(t *testing.T) {
	gormDB, err := db.Open("sqlite", t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		t.Fatal(err)
	}
	store := storage.NewGORMStorage(gormDB)
	reg := llm.NewRegistry(nil)
	claude, _ := llm.NewClaudeProvider()
	reg.Register(claude)
	hub := event.NewHub()
	exporters := observability.NewExporterRegistry()
	otel := observability.NewOTel(exporters)
	factory := agent.NewFactory(reg, hub, otel)
	art := artifact.NewService(store)
	sess := session.NewService(store)
	strat := strategy.NewService(factory, art)
	disc := discovery.NewService(factory, art)
	reviewer := delivery.NewReviewer(factory, art, store, hub)
	deliv := delivery.NewService(factory, art, nil, store, reviewer, hub, otel)
	wfDir := filepath.Join("..", "..", "..", "workflows")
	orch := orchestrator.NewService(store, orchestrator.NewPlanner(wfDir), orchestrator.NewScheduler(executor.NewLocalExecutor()),
		factory, strat, disc, deliv, hub, otel, executor.NewLocalExecutor())
	s, err := sess.Create(context.Background(), session.CreateRequest{Mode: entity.ModeStrategy, Input: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Run(context.Background(), s.SHA); err != nil {
		t.Fatal(err)
	}
	st, _ := orch.GetStatus(context.Background(), s.SHA)
	if st != entity.StateCompleted {
		t.Fatalf("expected completed, got %s", st)
	}
}
