package executor

import (
	"context"
	"time"
)

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

type Task struct {
	ID      string
	Fn      func(ctx context.Context) error
	Timeout time.Duration
}

type TaskHandle string

type Result struct {
	Status TaskStatus
	Err    error
}

type Executor interface {
	Submit(ctx context.Context, task Task) (TaskHandle, error)
	Wait(ctx context.Context, handle TaskHandle) (Result, error)
	Cancel(ctx context.Context, handle TaskHandle) error
	Status(ctx context.Context, handle TaskHandle) (TaskStatus, error)
}
