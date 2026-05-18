package executor

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type RestateExecutor struct{}

func NewRestateExecutor() *RestateExecutor { return &RestateExecutor{} }

func (r *RestateExecutor) Submit(_ context.Context, _ Task) (TaskHandle, error) {
	return "", errors.ErrNotImplemented
}

func (r *RestateExecutor) Wait(_ context.Context, _ TaskHandle) (Result, error) {
	return Result{}, errors.ErrNotImplemented
}

func (r *RestateExecutor) Cancel(_ context.Context, _ TaskHandle) error {
	return errors.ErrNotImplemented
}

func (r *RestateExecutor) Status(_ context.Context, _ TaskHandle) (TaskStatus, error) {
	return TaskFailed, errors.ErrNotImplemented
}
