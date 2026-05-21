// Package loop implements the Karakuri autonomous reasoning loop.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
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
	// ResumeStoredLoops re-launches non-completed loops persisted by a
	// previous server process. Bootstrap calls this once at startup so loops
	// continue across server restarts (Phase 11).
	ResumeStoredLoops(ctx context.Context) error
}

type loopState struct {
	id         string
	status     loop.Status
	result     loop.Result
	decisionCh chan corecheckpoint.Decision
	request    loop.Request // captured for durable replay on server restart
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
		request:    req,
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

	// Persist initial state so a server restart can identify + resume the loop.
	s.persistState(ctx, state, false)

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

// persistState writes the current loop state slice to durable storage. Called
// at every iteration boundary so a server crash never loses more than one
// iteration of progress. Storage errors are swallowed — in-memory state is
// always the source of truth for the running process; persistence is a
// best-effort durability layer.
func (s *serviceImpl) persistState(ctx context.Context, state *loopState, completed bool) {
	if state == nil {
		return
	}
	state.mu.RLock()
	status := state.status
	req := state.request
	result := state.result
	state.mu.RUnlock()

	reqJSON, _ := json.Marshal(req)
	cpID := ""
	if result.CheckpointID != nil {
		cpID = *result.CheckpointID
	}
	persisted := coreloop.State{
		LoopID:       state.id,
		ObjectiveID:  status.ObjectiveID,
		TwinID:       req.Twin.ID,
		AgentID:      string(req.Agent.ID),
		Iteration:    status.Iteration,
		Paused:       status.Paused,
		Completed:    completed,
		LastStep:     status.Step,
		Status:       result.Status,
		CriteriaMet:  status.CriteriaMet,
		CheckpointID: cpID,
		RequestJSON:  string(reqJSON),
	}
	_ = s.store.SaveLoopState(ctx, persisted)
}

// ResumeStoredLoops re-launches background goroutines for every non-completed
// loop state in storage. Invoked by bootstrap on server start; idempotent on a
// freshly-started server because no goroutines exist yet. Paused loops are
// re-registered in the in-memory map and stay waiting for a Resume() call;
// active loops are re-launched and replay observe → reason → decide from the
// next iteration (mid-iteration progress is lost, but iteration boundaries
// are durable).
func (s *serviceImpl) ResumeStoredLoops(ctx context.Context) error {
	states, err := s.store.ListActiveLoopStates(ctx)
	if err != nil {
		return err
	}
	for _, st := range states {
		var req loop.Request
		if err := json.Unmarshal([]byte(st.RequestJSON), &req); err != nil {
			continue // skip un-resumable rows but keep going
		}
		ls := &loopState{
			id:         st.LoopID,
			decisionCh: make(chan corecheckpoint.Decision, 1),
			request:    req,
			result: loop.Result{
				LoopID:      st.LoopID,
				ObjectiveID: st.ObjectiveID,
				Status:      st.Status,
			},
			status: loop.Status{
				LoopID:      st.LoopID,
				ObjectiveID: st.ObjectiveID,
				Step:        st.LastStep,
				Iteration:   st.Iteration,
				CriteriaMet: st.CriteriaMet,
				Paused:      st.Paused,
			},
		}
		s.mu.Lock()
		s.states[st.LoopID] = ls
		s.mu.Unlock()

		if !st.Paused {
			go s.runLoop(context.Background(), st.LoopID, req)
		}
	}
	return nil
}

// Resumer is the optional capability bootstrap looks for at startup to revive
// stored loops. Implementations may live elsewhere if the executor strategy
// changes (Restate, Celery, …) — Phase 11 implements it on the local-goroutine
// service.
type Resumer interface {
	ResumeStoredLoops(ctx context.Context) error
}

