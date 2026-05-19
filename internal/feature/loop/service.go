// Package loop implements the Karakuri autonomous reasoning loop.
// Phase 1: interface definition and stub. Full six-step implementation in Phase 2.
package loop

import (
	"context"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/loop"
)

// Service drives the observe→reason→decide→act→verify→learn loop.
type Service interface {
	Run(ctx context.Context, req loop.Request) (loop.Result, error)
	Resume(ctx context.Context, loopID string, decision corecheckpoint.Decision) (loop.Result, error)
	Status(ctx context.Context, loopID string) (loop.Status, error)
}

// stubService satisfies the interface during Phase 1.
type stubService struct{}

func NewService() Service { return &stubService{} }

func (s *stubService) Run(_ context.Context, req loop.Request) (loop.Result, error) {
	return loop.Result{
		ObjectiveID: req.Objective.ID,
		Status:      req.Objective.Status,
	}, errors.ErrNotImplemented
}

func (s *stubService) Resume(_ context.Context, _ string, _ corecheckpoint.Decision) (loop.Result, error) {
	return loop.Result{}, errors.ErrNotImplemented
}

func (s *stubService) Status(_ context.Context, _ string) (loop.Status, error) {
	return loop.Status{}, errors.ErrNotImplemented
}
