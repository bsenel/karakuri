package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakePruner struct {
	cutoffs []time.Time
	count   int64
	err     error
}

func (f *fakePruner) DeleteToolEventsBefore(_ context.Context, cutoff time.Time) (int64, error) {
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.count, f.err
}

func TestCheckRetention(t *testing.T) {
	tests := []struct {
		name string
		r    Retention
		// wantIn is empty when r is valid; otherwise every entry must
		// appear in the error message.
		wantIn []string
	}{
		{"never prune at the floor", Retention{FloorDays: 183, Days: 0}, nil},
		{"days equal to the floor", Retention{FloorDays: 183, Days: 183}, nil},
		{"raised floor, days above it", Retention{FloorDays: 365, Days: 400}, nil},
		{"floor lowered below the declared one", Retention{FloorDays: 100, Days: 0}, []string{"183", "100"}},
		{"days below the floor", Retention{FloorDays: 183, Days: 30}, []string{"183", "30"}},
		{"days below a raised floor", Retention{FloorDays: 365, Days: 200}, []string{"365", "200"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRetention(tt.r)
			if len(tt.wantIn) == 0 {
				if err != nil {
					t.Fatalf("CheckRetention(%+v) = %v, want nil", tt.r, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckRetention(%+v) = nil, want an error naming %v", tt.r, tt.wantIn)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

func TestPruneNeverWhenDaysIsZero(t *testing.T) {
	store := &fakePruner{count: 7}
	n, err := NewService(store, Retention{FloorDays: 183, Days: 0}).Prune(context.Background(), time.Now())
	if err != nil || n != 0 {
		t.Fatalf("Prune = %d, %v; want 0, nil", n, err)
	}
	if len(store.cutoffs) != 0 {
		t.Fatalf("store called with %v, want no calls", store.cutoffs)
	}
}

func TestPruneDeletesBeforeTheRetentionWindow(t *testing.T) {
	now := time.Date(2026, 10, 3, 9, 30, 0, 0, time.UTC)
	store := &fakePruner{count: 42}
	n, err := NewService(store, Retention{FloorDays: 183, Days: 200}).Prune(context.Background(), now)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 42 {
		t.Errorf("Prune = %d, want the store's count 42", n)
	}
	if len(store.cutoffs) != 1 {
		t.Fatalf("store called %d times, want exactly once", len(store.cutoffs))
	}
	if want := now.Add(-200 * 24 * time.Hour); !store.cutoffs[0].Equal(want) {
		t.Errorf("cutoff = %s, want %s", store.cutoffs[0], want)
	}
}

// The floor is enforced where the delete happens, not only where config is
// loaded: a service built directly with a below-floor retention deletes
// nothing.
func TestPruneRefusesBelowFloor(t *testing.T) {
	tests := []struct {
		name      string
		r         Retention
		wantFloor string
	}{
		{"days below the floor", Retention{FloorDays: 183, Days: 30}, "183"},
		{"floor lowered below the declared one", Retention{FloorDays: 100, Days: 150}, "183"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakePruner{count: 9}
			n, err := NewService(store, tt.r).Prune(context.Background(), time.Now())
			if err == nil {
				t.Fatalf("Prune with %+v = %d, nil; want an error", tt.r, n)
			}
			if !strings.Contains(err.Error(), tt.wantFloor) {
				t.Errorf("error %q does not name the floor %s", err, tt.wantFloor)
			}
			if n != 0 {
				t.Errorf("Prune = %d, want 0", n)
			}
			if len(store.cutoffs) != 0 {
				t.Errorf("store called with %v, want no calls", store.cutoffs)
			}
		})
	}
}

func TestPruneReturnsStoreError(t *testing.T) {
	boom := errors.New("disk on fire")
	store := &fakePruner{err: boom}
	_, err := NewService(store, Retention{FloorDays: 183, Days: 200}).Prune(context.Background(), time.Now())
	if !errors.Is(err, boom) {
		t.Fatalf("Prune error = %v, want %v", err, boom)
	}
}
