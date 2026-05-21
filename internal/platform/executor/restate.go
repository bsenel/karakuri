package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// RestateExecutor submits durable workflow invocations to a Restate ingress
// over HTTP. The implementation here is the client side: Karakuri picks a
// service path (`{ingress}/{service}/{handler}`), POSTs the task payload, and
// stores the invocation ID for later Wait()/Status() polls.
//
// The Restate side itself — registering the service that actually executes
// the task — is a separate deployment artifact (a Go/TS/Java service the
// operator runs alongside Restate). What this executor guarantees is the
// submit-and-track path: durable enqueue + idempotency token + status
// polling against the Restate ingress.
//
// Configuration:
//
//	RESTATE_INGRESS_URL   — e.g. http://localhost:8080 (Restate's HTTP ingress)
//	RESTATE_SERVICE       — service path, e.g. "Karakuri.Task"
//	RESTATE_HANDLER       — handler name, e.g. "Run"
//	RESTATE_AUTH_TOKEN    — optional Authorization: Bearer header
//
// When RESTATE_INGRESS_URL is unset the executor degrades to the local
// goroutine fallback so dev installs without Restate keep working.
type RestateExecutor struct {
	ingressURL string
	service    string
	handler    string
	authToken  string
	client     *http.Client
	fallback   *LocalExecutor // used when ingressURL is empty or unreachable

	mu    sync.RWMutex
	tasks map[TaskHandle]*restateInvocation
}

type restateInvocation struct {
	invocationID string
	status       TaskStatus
	err          error
	done         chan struct{}
}

func NewRestateExecutor() *RestateExecutor {
	return &RestateExecutor{
		ingressURL: strings.TrimRight(os.Getenv("RESTATE_INGRESS_URL"), "/"),
		service:    envDefault("RESTATE_SERVICE", "Karakuri.Task"),
		handler:    envDefault("RESTATE_HANDLER", "Run"),
		authToken:  os.Getenv("RESTATE_AUTH_TOKEN"),
		client:     &http.Client{Timeout: 30 * time.Second},
		fallback:   NewLocalExecutor(),
		tasks:      make(map[TaskHandle]*restateInvocation),
	}
}

// Active reports whether a Restate ingress is configured. /health uses this
// to surface "restate: configured" vs the local-fallback path.
func (r *RestateExecutor) Active() bool { return r.ingressURL != "" }

func (r *RestateExecutor) Submit(ctx context.Context, task Task) (TaskHandle, error) {
	if r.ingressURL == "" {
		return r.fallback.Submit(ctx, task)
	}

	body, _ := json.Marshal(map[string]any{"task_id": task.ID})
	url := fmt.Sprintf("%s/%s/%s", r.ingressURL, r.service, r.handler)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if task.ID != "" {
		req.Header.Set("idempotency-key", task.ID)
	}
	if r.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.authToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("restate: submit: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("restate: submit HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	invocationID := strings.TrimSpace(string(respBody))
	handle := TaskHandle(task.ID)
	r.mu.Lock()
	r.tasks[handle] = &restateInvocation{
		invocationID: invocationID,
		status:       TaskPending,
		done:         make(chan struct{}),
	}
	r.mu.Unlock()
	return handle, nil
}

func (r *RestateExecutor) Wait(ctx context.Context, handle TaskHandle) (Result, error) {
	if r.ingressURL == "" {
		return r.fallback.Wait(ctx, handle)
	}
	r.mu.RLock()
	inv, ok := r.tasks[handle]
	r.mu.RUnlock()
	if !ok {
		return Result{Status: TaskFailed}, fmt.Errorf("restate: unknown handle %q", handle)
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return Result{Status: TaskCancelled, Err: ctx.Err()}, nil
		case <-inv.done:
			return Result{Status: inv.status, Err: inv.err}, nil
		case <-ticker.C:
			st, err := r.Status(ctx, handle)
			if err == nil && (st == TaskCompleted || st == TaskFailed || st == TaskCancelled) {
				return Result{Status: st}, nil
			}
		}
	}
}

func (r *RestateExecutor) Cancel(ctx context.Context, handle TaskHandle) error {
	if r.ingressURL == "" {
		return r.fallback.Cancel(ctx, handle)
	}
	r.mu.RLock()
	inv, ok := r.tasks[handle]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("restate: unknown handle %q", handle)
	}
	url := fmt.Sprintf("%s/invocations/%s/cancel", r.ingressURL, inv.invocationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if r.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.authToken)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("restate: cancel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("restate: cancel HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	r.mu.Lock()
	inv.status = TaskCancelled
	close(inv.done)
	r.mu.Unlock()
	return nil
}

func (r *RestateExecutor) Status(ctx context.Context, handle TaskHandle) (TaskStatus, error) {
	if r.ingressURL == "" {
		return r.fallback.Status(ctx, handle)
	}
	r.mu.RLock()
	inv, ok := r.tasks[handle]
	r.mu.RUnlock()
	if !ok {
		return TaskFailed, fmt.Errorf("restate: unknown handle %q", handle)
	}
	url := fmt.Sprintf("%s/invocations/%s", r.ingressURL, inv.invocationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TaskFailed, err
	}
	if r.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.authToken)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return TaskFailed, fmt.Errorf("restate: status: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return TaskFailed, fmt.Errorf("restate: invocation %s not found", inv.invocationID)
	}
	if resp.StatusCode >= 400 {
		return TaskFailed, fmt.Errorf("restate: status HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var view struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &view); err != nil {
		return TaskFailed, fmt.Errorf("restate: parse status: %w", err)
	}
	switch strings.ToLower(view.Status) {
	case "succeeded", "completed":
		return TaskCompleted, nil
	case "failed":
		return TaskFailed, nil
	case "cancelled":
		return TaskCancelled, nil
	case "running":
		return TaskRunning, nil
	default:
		return TaskPending, nil
	}
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
