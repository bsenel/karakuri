package executor

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/platform/valkey"
)

// CeleryExecutor publishes tasks to a Celery v2 message queue on a Redis
// broker. Tasks are pushed onto Redis lists keyed by queue name; Celery
// workers (separate Python processes the operator runs alongside Karakuri)
// pop, execute, and write results to a result backend.
//
// The Redis protocol implementation is intentionally minimal — just enough
// RPUSH / GET to ship tasks and read back result entries. This avoids
// pulling in a full Redis client dependency for what's a niche execution
// surface; if operators want richer Celery support they can run the
// Karakuri-side service through go-redis.
//
// Configuration:
//
//	CELERY_BROKER_URL  — redis://[:password@]host:port/db  (default: redis://localhost:6379/0)
//	CELERY_QUEUE       — queue name (default: "celery")
//	CELERY_TASK_NAME   — task name workers register (default: "karakuri.run_task")
//
// When CELERY_BROKER_URL is unset (and no broker is reachable on the
// default host/port) the executor degrades to the local fallback so dev
// installs without Redis keep working.
type CeleryExecutor struct {
	brokerURL string
	queue     string
	taskName  string
	fallback  *LocalExecutor

	// client is the pooled broker connection, built lazily by broker().
	client *valkey.Client

	mu    sync.RWMutex
	tasks map[TaskHandle]*celeryInvocation
}

type celeryInvocation struct {
	celeryID string
	status   TaskStatus
}

func NewCeleryExecutor() *CeleryExecutor {
	return &CeleryExecutor{
		brokerURL: envDefault("CELERY_BROKER_URL", ""),
		queue:     envDefault("CELERY_QUEUE", "celery"),
		taskName:  envDefault("CELERY_TASK_NAME", "karakuri.run_task"),
		fallback:  NewLocalExecutor(),
		tasks:     make(map[TaskHandle]*celeryInvocation),
	}
}

// Active reports whether a broker URL is configured. /health uses this to
// surface "celery: configured" vs the local-fallback path.
func (c *CeleryExecutor) Active() bool { return c.brokerURL != "" }

func (c *CeleryExecutor) Submit(ctx context.Context, task Task) (TaskHandle, error) {
	if c.brokerURL == "" {
		return c.fallback.Submit(ctx, task)
	}
	celeryID := newCeleryID()
	envelope := buildCeleryMessage(celeryID, c.taskName, []any{task.ID}, nil)

	client, err := c.broker()
	if err != nil {
		return "", fmt.Errorf("celery: broker: %w", err)
	}
	if _, err := client.Do(ctx, "RPUSH", c.queue, envelope); err != nil {
		return "", fmt.Errorf("celery: rpush: %w", err)
	}

	handle := TaskHandle(task.ID)
	c.mu.Lock()
	c.tasks[handle] = &celeryInvocation{celeryID: celeryID, status: TaskPending}
	c.mu.Unlock()
	return handle, nil
}

func (c *CeleryExecutor) Wait(ctx context.Context, handle TaskHandle) (Result, error) {
	if c.brokerURL == "" {
		return c.fallback.Wait(ctx, handle)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return Result{Status: TaskCancelled, Err: ctx.Err()}, nil
		case <-ticker.C:
			st, err := c.Status(ctx, handle)
			if err == nil && (st == TaskCompleted || st == TaskFailed || st == TaskCancelled) {
				return Result{Status: st}, nil
			}
		}
	}
}

func (c *CeleryExecutor) Cancel(_ context.Context, _ TaskHandle) error {
	// Celery's revoke API requires the celery control protocol (which we
	// don't bother implementing). Operators can use `celery control revoke`
	// out of band; this method is a no-op for the Karakuri client.
	return fmt.Errorf("celery: cancel not supported by this minimal client; use celery control revoke <id>")
}

func (c *CeleryExecutor) Status(ctx context.Context, handle TaskHandle) (TaskStatus, error) {
	if c.brokerURL == "" {
		return c.fallback.Status(ctx, handle)
	}
	c.mu.RLock()
	inv, ok := c.tasks[handle]
	c.mu.RUnlock()
	if !ok {
		return TaskFailed, fmt.Errorf("celery: unknown handle %q", handle)
	}

	// Celery writes results to `celery-task-meta-<id>` in the result backend.
	// We assume the result backend == the broker (common for Redis).
	client, err := c.broker()
	if err != nil {
		return TaskPending, err
	}
	reply, err := client.Do(ctx, "GET", "celery-task-meta-"+inv.celeryID)
	if err != nil {
		return TaskPending, nil // result not yet posted
	}
	val, _ := reply.(string)
	if val == "" {
		return TaskPending, nil
	}

	var meta struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		return TaskPending, nil
	}
	switch strings.ToUpper(meta.Status) {
	case "SUCCESS":
		return TaskCompleted, nil
	case "FAILURE":
		return TaskFailed, nil
	case "REVOKED":
		return TaskCancelled, nil
	case "STARTED":
		return TaskRunning, nil
	default:
		return TaskPending, nil
	}
}

// ── Celery v2 message envelope ───────────────────────────────────────────────

// buildCeleryMessage produces a base64-encoded JSON envelope matching Celery's
// v2 message protocol. Workers configured with the standard `json` serializer
// pop, decode, and dispatch by `task` name.
func buildCeleryMessage(id, taskName string, args []any, kwargs map[string]any) string {
	if kwargs == nil {
		kwargs = map[string]any{}
	}
	headers := map[string]any{
		"id":         id,
		"task":       taskName,
		"lang":       "py",
		"shadow":     nil,
		"eta":        nil,
		"retries":    0,
		"timelimit":  []any{nil, nil},
		"argsrepr":   fmt.Sprintf("(%s,)", joinArgs(args)),
		"kwargsrepr": "{}",
		"origin":     "karakuri",
	}
	properties := map[string]any{
		"correlation_id": id,
		"reply_to":       "",
		"delivery_mode":  2,
		"delivery_info":  map[string]any{"exchange": "", "routing_key": "celery"},
		"priority":       0,
		"body_encoding":  "base64",
		"delivery_tag":   id,
	}
	body, _ := json.Marshal([]any{args, kwargs, map[string]any{"callbacks": nil, "errbacks": nil, "chain": nil, "chord": nil}})
	bodyB64 := base64.StdEncoding.EncodeToString(body)

	envelope := map[string]any{
		"body":             bodyB64,
		"content-encoding": "utf-8",
		"content-type":     "application/json",
		"headers":          headers,
		"properties":       properties,
	}
	out, _ := json.Marshal(envelope)
	return string(out)
}

func joinArgs(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			parts[i] = "'" + v + "'"
		default:
			parts[i] = fmt.Sprintf("%v", v)
		}
	}
	return strings.Join(parts, ", ")
}

func newCeleryID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// broker returns the pooled connection to the result backend, built on first
// use so a broker that is not up yet does not stop the process from starting.
//
// The client itself lives in internal/platform/valkey, shared with the quota
// limiter. It replaces the RESP implementation that used to sit at the bottom
// of this file — which dialled per call, and sized its reads from whatever
// length the server claimed.
func (c *CeleryExecutor) broker() (*valkey.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	client, err := valkey.New(c.brokerURL, valkey.Options{})
	if err != nil {
		return nil, err
	}
	c.client = client
	return client, nil
}
