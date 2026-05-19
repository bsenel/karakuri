// Package loop implements the Karakuri autonomous reasoning loop.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureart "github.com/bsenel/karakuri/internal/feature/artifact"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Service drives the observe→reason→decide→act→verify→learn loop.
type Service interface {
	Run(ctx context.Context, req loop.Request) (loop.Result, error)
	Resume(ctx context.Context, loopID string, decision corecheckpoint.Decision) (loop.Result, error)
	Status(ctx context.Context, loopID string) (loop.Status, error)
}

type loopState struct {
	id         string
	status     loop.Status
	result     loop.Result
	decisionCh chan corecheckpoint.Decision
	mu         sync.RWMutex
}

type serviceImpl struct {
	store   storage.StorageAdapter
	factory *platformagent.Factory
	capReg  *capability.Registry
	envReg  *environment.Registry
	memSvc  *featurememory.Service
	cpSvc   *featurecp.Service
	artSvc  *featureart.Service
	wt      git.WorktreeManager
	hub     *event.Hub
	otel    *observability.OTel
	domReg  *domain.Registry

	mu     sync.RWMutex
	states map[string]*loopState // loopID → state
}

func NewService(
	store storage.StorageAdapter,
	factory *platformagent.Factory,
	capReg *capability.Registry,
	envReg *environment.Registry,
	memSvc *featurememory.Service,
	cpSvc *featurecp.Service,
	artSvc *featureart.Service,
	wt git.WorktreeManager,
	hub *event.Hub,
	otel *observability.OTel,
	domReg *domain.Registry,
) Service {
	return &serviceImpl{
		store:   store,
		factory: factory,
		capReg:  capReg,
		envReg:  envReg,
		memSvc:  memSvc,
		cpSvc:   cpSvc,
		artSvc:  artSvc,
		wt:      wt,
		hub:     hub,
		otel:    otel,
		domReg:  domReg,
		states:  make(map[string]*loopState),
	}
}

func (s *serviceImpl) Run(ctx context.Context, req loop.Request) (loop.Result, error) {
	id, err := newLoopID()
	if err != nil {
		return loop.Result{}, fmt.Errorf("generate loop id: %w", err)
	}

	state := &loopState{
		id:         id,
		decisionCh: make(chan corecheckpoint.Decision, 1),
		result: loop.Result{
			LoopID:      id,
			ObjectiveID: req.Objective.ID,
			Status:      objective.StatusActive,
		},
		status: loop.Status{
			LoopID:      id,
			ObjectiveID: req.Objective.ID,
			Step:        loop.StepObserve,
			Iteration:   0,
		},
	}

	s.mu.Lock()
	s.states[id] = state
	s.mu.Unlock()

	// Run the loop in background goroutine
	go s.runLoop(context.Background(), id, req)

	return loop.Result{
		LoopID:      id,
		ObjectiveID: req.Objective.ID,
		Status:      objective.StatusActive,
	}, nil
}

func (s *serviceImpl) Resume(ctx context.Context, loopID string, decision corecheckpoint.Decision) (loop.Result, error) {
	s.mu.RLock()
	state, ok := s.states[loopID]
	s.mu.RUnlock()
	if !ok {
		return loop.Result{}, fmt.Errorf("loop %q not found", loopID)
	}

	// Send decision non-blocking (buffer size 1)
	select {
	case state.decisionCh <- decision:
	default:
		return loop.Result{}, fmt.Errorf("loop %q is not waiting for a decision", loopID)
	}

	// Return current result
	state.mu.RLock()
	result := state.result
	state.mu.RUnlock()
	return result, nil
}

func (s *serviceImpl) Status(ctx context.Context, loopID string) (loop.Status, error) {
	s.mu.RLock()
	state, ok := s.states[loopID]
	s.mu.RUnlock()
	if !ok {
		return loop.Status{}, fmt.Errorf("loop %q not found", loopID)
	}

	state.mu.RLock()
	status := state.status
	state.mu.RUnlock()
	return status, nil
}

func newLoopID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

