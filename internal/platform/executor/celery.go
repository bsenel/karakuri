package executor

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
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

	mu    sync.RWMutex
	tasks map[TaskHandle]*celeryInvocation
}

type celeryInvocation struct {
	celeryID string
	status   TaskStatus
	err      error
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

	conn, err := dialRedis(ctx, c.brokerURL)
	if err != nil {
		return "", fmt.Errorf("celery: dial broker: %w", err)
	}
	defer conn.Close()

	// RPUSH <queue> <message>
	if err := redisCmd(conn, "RPUSH", c.queue, envelope); err != nil {
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
	conn, err := dialRedis(ctx, c.brokerURL)
	if err != nil {
		return TaskPending, err
	}
	defer conn.Close()
	key := "celery-task-meta-" + inv.celeryID
	val, err := redisGet(conn, key)
	if err != nil {
		return TaskPending, nil // result not yet posted
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
		"id":     id,
		"task":   taskName,
		"lang":   "py",
		"shadow": nil,
		"eta":    nil,
		"retries": 0,
		"timelimit": []any{nil, nil},
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

// ── Minimal Redis client (RESP protocol over TCP) ───────────────────────────

// dialRedis parses a redis:// URL and opens a TCP connection. The minimal
// client speaks the RESP protocol directly — enough for RPUSH + GET, which
// are the only commands we issue. Production deployments that need richer
// Redis usage can run with go-redis instead by swapping this executor.
func dialRedis(ctx context.Context, brokerURL string) (net.Conn, error) {
	u, err := url.Parse(brokerURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":6379"
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	// AUTH if password supplied
	if pw, ok := u.User.Password(); ok && pw != "" {
		if err := redisCmd(conn, "AUTH", pw); err != nil {
			conn.Close()
			return nil, fmt.Errorf("auth: %w", err)
		}
	}
	// SELECT db
	dbStr := strings.TrimPrefix(u.Path, "/")
	if dbStr != "" {
		if _, err := strconv.Atoi(dbStr); err == nil {
			if err := redisCmd(conn, "SELECT", dbStr); err != nil {
				conn.Close()
				return nil, fmt.Errorf("select db: %w", err)
			}
		}
	}
	return conn, nil
}

// redisCmd writes a RESP-encoded command and reads/discards the simple reply.
// Errors include RESP `-ERR` replies.
func redisCmd(conn net.Conn, args ...string) error {
	if err := writeRESPArray(conn, args); err != nil {
		return err
	}
	_, err := readRESPReply(bufio.NewReader(conn))
	return err
}

// redisGet issues GET <key> and returns the bulk-string reply.
func redisGet(conn net.Conn, key string) (string, error) {
	if err := writeRESPArray(conn, []string{"GET", key}); err != nil {
		return "", err
	}
	v, err := readRESPReply(bufio.NewReader(conn))
	if err != nil {
		return "", err
	}
	return v, nil
}

func writeRESPArray(conn net.Conn, args []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	_, err := conn.Write([]byte(b.String()))
	return err
}

func readRESPReply(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 3 {
		return "", fmt.Errorf("short reply: %q", line)
	}
	switch line[0] {
	case '+': // simple string
		return strings.TrimRight(line[1:], "\r\n"), nil
	case '-': // error
		return "", fmt.Errorf("redis: %s", strings.TrimRight(line[1:], "\r\n"))
	case ':': // integer
		return strings.TrimRight(line[1:], "\r\n"), nil
	case '$': // bulk string
		n, err := strconv.Atoi(strings.TrimRight(line[1:], "\r\n"))
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", fmt.Errorf("nil")
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	default:
		return "", fmt.Errorf("unsupported reply: %s", line)
	}
}

