package executor

import (
	"context"
	"sync"
	"time"
)

type LocalExecutor struct {
	mu      sync.Mutex
	tasks   map[TaskHandle]*localTask
	counter int
}

type localTask struct {
	status TaskStatus
	result error
	done   chan struct{}
	cancel context.CancelFunc
}

func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{tasks: make(map[TaskHandle]*localTask)}
}

func (e *LocalExecutor) Submit(ctx context.Context, task Task) (TaskHandle, error) {
	e.mu.Lock()
	e.counter++
	handle := TaskHandle(task.ID)
	if handle == "" {
		handle = TaskHandle(time.Now().Format("20060102150405"))
	}
	lt := &localTask{status: TaskPending, done: make(chan struct{})}
	e.tasks[handle] = lt
	e.mu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	lt.cancel = cancel
	if task.Timeout > 0 {
		var cancelTimeout context.CancelFunc
		runCtx, cancelTimeout = context.WithTimeout(runCtx, task.Timeout)
		defer cancelTimeout()
	}

	go func() {
		e.mu.Lock()
		lt.status = TaskRunning
		e.mu.Unlock()
		err := task.Fn(runCtx)
		e.mu.Lock()
		if err != nil {
			lt.status = TaskFailed
			lt.result = err
		} else {
			lt.status = TaskCompleted
		}
		e.mu.Unlock()
		close(lt.done)
	}()
	return handle, nil
}

func (e *LocalExecutor) Wait(ctx context.Context, handle TaskHandle) (Result, error) {
	e.mu.Lock()
	lt, ok := e.tasks[handle]
	e.mu.Unlock()
	if !ok {
		return Result{Status: TaskFailed}, nil
	}
	select {
	case <-lt.done:
		return Result{Status: lt.status, Err: lt.result}, nil
	case <-ctx.Done():
		return Result{Status: TaskCancelled, Err: ctx.Err()}, nil
	}
}

func (e *LocalExecutor) Cancel(_ context.Context, handle TaskHandle) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if lt, ok := e.tasks[handle]; ok && lt.cancel != nil {
		lt.cancel()
		lt.status = TaskCancelled
	}
	return nil
}

func (e *LocalExecutor) Status(_ context.Context, handle TaskHandle) (TaskStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if lt, ok := e.tasks[handle]; ok {
		return lt.status, nil
	}
	return TaskFailed, nil
}
