package event

import (
	"context"
	"sync"
	"time"
)

type Type string

const (
	AgentStarted     Type = "agent_started"
	ArtifactWritten  Type = "artifact_written"
	WorktreeCreated  Type = "worktree_created"
	WorktreeRemoved  Type = "worktree_removed"
	WorktreePruned   Type = "worktree_pruned"
	Checkpoint       Type = "checkpoint"
	ReviewCompleted  Type = "review_completed"
	TaskFailed       Type = "task_failed"
	AdapterSkipped   Type = "adapter_skipped"
	ActionItem       Type = "action_item"
	PromotionReady   Type = "promotion_ready"
	SessionCompleted Type = "session_completed"
)

type Event struct {
	Type       Type           `json:"type"`
	SessionSHA string         `json:"session_sha,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	Timestamp  time.Time      `json:"ts"`
}

type Publisher interface {
	Publish(ctx context.Context, evt Event) error
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[string]map[chan Event]struct{})}
}

func (h *Hub) Publish(_ context.Context, evt Event) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	key := evt.SessionSHA
	if key == "" {
		key = "_global"
	}
	for ch := range h.subscribers[key] {
		select {
		case ch <- evt:
		default:
		}
	}
	return nil
}

func (h *Hub) Subscribe(_ context.Context, sessionSHA string) (<-chan Event, func(), error) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	if h.subscribers[sessionSHA] == nil {
		h.subscribers[sessionSHA] = make(map[chan Event]struct{})
	}
	h.subscribers[sessionSHA][ch] = struct{}{}
	h.mu.Unlock()
	unsub := func() {
		h.mu.Lock()
		delete(h.subscribers[sessionSHA], ch)
		close(ch)
		h.mu.Unlock()
	}
	return ch, unsub, nil
}
