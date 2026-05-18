package orchestrator

import (
	"context"
	"sync"

	"github.com/bsenel/karakuri/internal/platform/executor"
)

type Scheduler struct {
	exec executor.Executor
}

func NewScheduler(exec executor.Executor) *Scheduler {
	return &Scheduler{exec: exec}
}

type TaskRunner func(ctx context.Context, task AgentTask) error

func (s *Scheduler) RunParallel(ctx context.Context, tasks []AgentTask, runner TaskRunner) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))
	for _, task := range tasks {
		if !task.Parallel {
			if err := runner(ctx, task); err != nil {
				return err
			}
			continue
		}
		wg.Add(1)
		t := task
		handle, err := s.exec.Submit(ctx, executor.Task{
			ID: t.ID,
			Fn: func(ctx context.Context) error {
				defer wg.Done()
				return runner(ctx, t)
			},
		})
		if err != nil {
			wg.Done()
			return err
		}
		go func() {
			result, _ := s.exec.Wait(ctx, handle)
			if result.Err != nil {
				errCh <- result.Err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) RunSerial(ctx context.Context, tasks []AgentTask, runner TaskRunner) error {
	for _, task := range tasks {
		if err := runner(ctx, task); err != nil {
			return err
		}
	}
	return nil
}
