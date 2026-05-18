package executor

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type CeleryExecutor struct{}

func NewCeleryExecutor() *CeleryExecutor { return &CeleryExecutor{} }

func (c *CeleryExecutor) Submit(_ context.Context, _ Task) (TaskHandle, error) {
	return "", errors.ErrNotImplemented
}

func (c *CeleryExecutor) Wait(_ context.Context, _ TaskHandle) (Result, error) {
	return Result{}, errors.ErrNotImplemented
}

func (c *CeleryExecutor) Cancel(_ context.Context, _ TaskHandle) error {
	return errors.ErrNotImplemented
}

func (c *CeleryExecutor) Status(_ context.Context, _ TaskHandle) (TaskStatus, error) {
	return TaskFailed, errors.ErrNotImplemented
}
