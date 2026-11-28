package gateway

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Phase 39 step 1: every tool call, from the loop or from the served MCP
// surface, takes one capability charge, writes one audit row and records one
// cost entry, all in the gateway; a call the quota refuses never runs; and a
// result's provenance survives the trip.

// auditStore records the audit rows the gateway writes. Every other storage
// method is the nil embedded interface, so a gateway that reaches for one
// fails the test loudly instead of passing by accident.
type auditStore struct {
	storage.StorageAdapter
	mu   sync.Mutex
	rows []storage.ToolEvent
}

func (s *auditStore) SaveToolEvent(_ context.Context, e storage.ToolEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, e)
	return nil
}

func (s *auditStore) saved() []storage.ToolEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storage.ToolEvent(nil), s.rows...)
}

// countingBackend admits or refuses every take, and counts them.
type countingBackend struct {
	allow bool
	mu    sync.Mutex
	takes []quota.Key
}

func (b *countingBackend) Take(_ context.Context, key quota.Key, p quota.Policy, n int, _ time.Time) (quota.Decision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.takes = append(b.takes, key)
	if !b.allow {
		return quota.Decision{Allowed: false, Limit: 1, Remaining: 0}, nil
	}
	return quota.Decision{Allowed: true, Limit: 1000, Remaining: 999}, nil
}

func (b *countingBackend) Peek(context.Context, quota.Key, quota.Policy, time.Time) (quota.Decision, error) {
	return quota.Decision{Allowed: b.allow}, nil
}

func (b *countingBackend) Reset(context.Context, quota.Key) error { return nil }

func (b *countingBackend) taken() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.takes)
}

// countingLedger keeps every cost event recorded.
type countingLedger struct {
	mu     sync.Mutex
	events []cost.Event
}

func (l *countingLedger) Record(_ context.Context, e cost.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	return nil
}

func (l *countingLedger) Aggregate(context.Context, cost.Query) ([]cost.Bucket, error) {
	return nil, nil
}

func (l *countingLedger) recorded() []cost.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]cost.Event(nil), l.events...)
}

type harness struct {
	gw      *Gateway
	store   *auditStore
	backend *countingBackend
	ledger  *countingLedger
}

func newHarness(allow bool) harness {
	h := harness{
		store:   &auditStore{},
		backend: &countingBackend{allow: allow},
		ledger:  &countingLedger{},
	}
	costs := &karakuriquota.Recorder{Ledger: h.ledger}
	deps := karakuriquota.Deps{
		Backend: h.backend,
		TierSet: karakuriquota.NewTierResolver(karakuriquota.DefaultTiers(config.QuotaConfig{}), nil),
		Costs:   costs,
	}
	h.gw = New(h.store, deps, costs)
	return h
}

func callFrom(caller Caller) Call {
	return Call{
		Caller:      caller,
		TwinID:      "twin-1",
		ObjectiveID: "obj-1",
		AgentID:     "agent-1",
		Capability:  "software.observe.fetch_prs",
		EnvID:       "software.env.vcs",
		Params:      map[string]any{"state": "open"},
	}
}

var callers = []Caller{CallerLoop, CallerMCP}

func TestAnAllowedCallIsOneAuditRowOneQuotaChargeAndOneCostEntry(t *testing.T) {
	for _, caller := range callers {
		t.Run(string(caller), func(t *testing.T) {
			h := newHarness(true)
			call := callFrom(caller)
			ran := 0

			result, err := h.gw.Call(context.Background(), call, func(context.Context) (environment.ActionResult, error) {
				ran++
				return environment.ActionResult{Success: true}, nil
			})
			if err != nil {
				t.Fatalf("an allowed call returned an error: %v", err)
			}
			if !result.Success {
				t.Errorf("result = %+v, want the success the call returned", result)
			}
			if ran != 1 {
				t.Errorf("the call ran %d times, want once", ran)
			}

			if got := h.backend.taken(); got != 1 {
				t.Errorf("capability quota charges = %d, want exactly one", got)
			}
			if len(h.backend.takes) > 0 && !strings.Contains(string(h.backend.takes[0]), call.Capability) {
				t.Errorf("the charge was keyed %q, which does not name the capability %q", h.backend.takes[0], call.Capability)
			}

			rows := h.store.saved()
			if len(rows) != 1 {
				t.Fatalf("audit rows = %d, want exactly one", len(rows))
			}
			row := rows[0]
			if row.Capability != call.Capability || row.ObjectiveID != call.ObjectiveID ||
				row.AgentID != call.AgentID || row.Adapter != call.EnvID {
				t.Errorf("audit row %+v does not name the call %+v", row, call)
			}
			if !row.Success {
				t.Error("the audit row records a failure for a call that succeeded")
			}
			if !strings.Contains(row.PayloadJSON, string(caller)) {
				t.Errorf("audit payload %s does not say the call came from %q", row.PayloadJSON, caller)
			}

			events := h.ledger.recorded()
			if len(events) != 1 {
				t.Fatalf("cost entries = %d, want exactly one", len(events))
			}
			e := events[0]
			if e.Subject != karakuriquota.CostSubject(call.TwinID) {
				t.Errorf("cost subject = %q, want the twin %q", e.Subject, call.TwinID)
			}
			if e.ResourceType != "objective" || e.ResourceID != call.ObjectiveID {
				t.Errorf("cost resource = %s/%s, want objective/%s, as stepAct attributes it", e.ResourceType, e.ResourceID, call.ObjectiveID)
			}
			if e.Provider != call.EnvID || e.UnitKind != cost.UnitCalls || e.Units != 1 {
				t.Errorf("cost entry = %+v, want one call on %s", e, call.EnvID)
			}
		})
	}
}

func TestACallTheQuotaRefusesNeverRunsAndChargesNothingElse(t *testing.T) {
	for _, caller := range callers {
		t.Run(string(caller), func(t *testing.T) {
			h := newHarness(false)
			ran := false

			result, _ := h.gw.Call(context.Background(), callFrom(caller), func(context.Context) (environment.ActionResult, error) {
				ran = true
				return environment.ActionResult{Success: true}, nil
			})
			if ran {
				t.Fatal("the gateway ran a call the capability quota refused")
			}
			if result.Success {
				t.Error("a refused call reports success")
			}
			// The words stepAct writes today, so an operator reading the
			// outcome sees the same refusal whichever door the call came in by.
			if result.Error != "capability quota exhausted for the day" {
				t.Errorf("refusal = %q, want %q", result.Error, "capability quota exhausted for the day")
			}
			if got := h.backend.taken(); got != 1 {
				t.Errorf("quota takes = %d, want the one that was refused", got)
			}
			if events := h.ledger.recorded(); len(events) != 0 {
				t.Errorf("a refused call recorded cost: %+v", events)
			}
			for _, row := range h.store.saved() {
				if row.Success {
					t.Errorf("a refused call wrote a successful audit row: %+v", row)
				}
			}
		})
	}
}

func TestAThirdPartyResultKeepsItsTrust(t *testing.T) {
	cases := []struct {
		name  string
		trust environment.Trust
	}{
		{"third party", environment.TrustThirdParty},
		{"unstated", ""},
	}
	for _, caller := range callers {
		for _, tc := range cases {
			t.Run(string(caller)+"/"+tc.name, func(t *testing.T) {
				h := newHarness(true)
				result, err := h.gw.Call(context.Background(), callFrom(caller), func(context.Context) (environment.ActionResult, error) {
					return environment.ActionResult{Success: true, StateDelta: map[string]any{"title": "a pull request title somebody wrote"}, Trust: tc.trust}, nil
				})
				if err != nil {
					t.Fatalf("call: %v", err)
				}
				if result.Trust != tc.trust {
					t.Errorf("trust = %q, want %q as the call returned it", result.Trust, tc.trust)
				}
				if result.StateDelta["title"] != "a pull request title somebody wrote" {
					t.Errorf("state delta = %v, want the call's own", result.StateDelta)
				}
			})
		}
	}
}
