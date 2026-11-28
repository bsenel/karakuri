package loop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
)

// Phase 39 step 1: an action the loop runs goes through the gateway, and the
// gateway, not stepAct, writes its audit row and takes its capability charge.
//
// The gateway is built over a store that records what passes through it and
// a quota backend of its own, both separate from the loop's, so the test can
// tell who wrote what: a row the loop wrote itself reaches the shared store
// without passing the gateway's recorder, and a charge the loop took itself
// lands on the loop's backend.

// gatewayAuditStore is the loop's own store as the gateway sees it, recording
// the audit rows written through it.
type gatewayAuditStore struct {
	storage.StorageAdapter
	mu   sync.Mutex
	rows []storage.ToolEvent
}

func (s *gatewayAuditStore) SaveToolEvent(ctx context.Context, e storage.ToolEvent) error {
	s.mu.Lock()
	s.rows = append(s.rows, e)
	s.mu.Unlock()
	return s.StorageAdapter.SaveToolEvent(ctx, e)
}

func (s *gatewayAuditStore) saved() []storage.ToolEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storage.ToolEvent(nil), s.rows...)
}

// takeCounter admits every capability charge and counts them.
type takeCounter struct {
	mu    sync.Mutex
	takes int
}

func (b *takeCounter) Take(context.Context, quota.Key, quota.Policy, int, time.Time) (quota.Decision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.takes++
	return quota.Decision{Allowed: true, Limit: 1000, Remaining: 999}, nil
}

func (b *takeCounter) Peek(context.Context, quota.Key, quota.Policy, time.Time) (quota.Decision, error) {
	return quota.Decision{Allowed: true}, nil
}

func (b *takeCounter) Reset(context.Context, quota.Key) error { return nil }

func (b *takeCounter) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.takes
}

func quotaOver(backend quota.Backend) karakuriquota.Deps {
	return karakuriquota.Deps{
		Backend: backend,
		TierSet: karakuriquota.NewTierResolver(karakuriquota.DefaultTiers(config.QuotaConfig{}), nil),
	}
}

func TestAnActionThroughTheLoopIsOneAuditRowAndOneChargeWrittenByTheGateway(t *testing.T) {
	ctx := context.Background()
	sc := actFixture(t, &violatingEnv{})

	loopQuota := &takeCounter{}
	sc.svc.quota = quotaOver(loopQuota)

	gwStore := &gatewayAuditStore{StorageAdapter: sc.svc.store}
	gwQuota := &takeCounter{}
	sc.svc.gateway = gateway.New(gwStore, quotaOver(gwQuota), sc.svc.costs)

	outcomes := stepAct(ctx, sc, delegatePlan())
	if len(outcomes) != 1 || !outcomes[0].Result.Success {
		t.Fatalf("outcomes = %+v, want the one action, succeeded", outcomes)
	}

	all, err := sc.svc.store.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var rows []storage.ToolEvent
	for _, r := range all {
		if r.Capability == violatingCap && !r.BoundsViolation {
			rows = append(rows, r)
		}
	}
	if len(rows) != 1 {
		t.Errorf("audit rows for the action = %d, want exactly one", len(rows))
	}
	if through := gwStore.saved(); len(through) != 1 || through[0].Capability != violatingCap {
		t.Errorf("rows written through the gateway = %+v, want the action's one row", through)
	}

	if got := gwQuota.count(); got != 1 {
		t.Errorf("capability charges taken by the gateway = %d, want exactly one", got)
	}
	if got := loopQuota.count(); got != 0 {
		t.Errorf("capability charges stepAct took itself = %d, want none: the gateway takes them", got)
	}
}
