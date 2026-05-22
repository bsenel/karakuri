package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
)

// fakeMemoryBackend captures Forget calls so we can assert on them without
// touching a real database.
type fakeMemoryBackend struct {
	storeFn  func(context.Context, memory.Entry) error
	forgetFn func(context.Context, memory.RetentionPolicy) error
	last     memory.RetentionPolicy
	called   bool
}

func (f *fakeMemoryBackend) Store(ctx context.Context, e memory.Entry) error {
	if f.storeFn != nil {
		return f.storeFn(ctx, e)
	}
	return nil
}
func (f *fakeMemoryBackend) Recall(_ context.Context, _ memory.Query) ([]memory.Entry, error) {
	return nil, nil
}
func (f *fakeMemoryBackend) Forget(ctx context.Context, p memory.RetentionPolicy) error {
	f.called = true
	f.last = p
	if f.forgetFn != nil {
		return f.forgetFn(ctx, p)
	}
	return nil
}
func (f *fakeMemoryBackend) Consolidate(_ context.Context, _ agent.AgentID) error { return nil }

func TestIsEmptyPolicy(t *testing.T) {
	if !isEmptyPolicy(memory.RetentionPolicy{}) {
		t.Errorf("zero policy should be treated as empty")
	}
	before := time.Now()
	if isEmptyPolicy(memory.RetentionPolicy{Before: &before}) {
		t.Errorf("policy with Before should not be empty")
	}
	if isEmptyPolicy(memory.RetentionPolicy{MinScore: 0.5}) {
		t.Errorf("policy with MinScore should not be empty")
	}
}

func TestRunRetention_SkipsEmptyPolicies(t *testing.T) {
	semantic := &fakeMemoryBackend{}
	s := &Service{semantic: semantic}
	if err := s.RunRetention(context.Background(), RetentionPolicySet{}); err != nil {
		t.Fatalf("RunRetention returned error: %v", err)
	}
	if semantic.called {
		t.Errorf("expected semantic.Forget not to be called for empty policy set")
	}
}

func TestRunRetention_AppliesSemanticPolicy(t *testing.T) {
	semantic := &fakeMemoryBackend{}
	s := &Service{semantic: semantic}
	before := time.Now().Add(-72 * time.Hour)
	set := RetentionPolicySet{
		Semantic: memory.RetentionPolicy{Before: &before, MinScore: 0.3},
	}
	if err := s.RunRetention(context.Background(), set); err != nil {
		t.Fatalf("RunRetention: %v", err)
	}
	if !semantic.called {
		t.Fatalf("expected semantic.Forget to be called")
	}
	if semantic.last.MinScore != 0.3 {
		t.Errorf("expected MinScore 0.3, got %v", semantic.last.MinScore)
	}
	if semantic.last.Before == nil || !semantic.last.Before.Equal(before) {
		t.Errorf("expected Before %v, got %v", before, semantic.last.Before)
	}
}

func TestRunRetention_ContinuesOnTierFailure(t *testing.T) {
	semantic := &fakeMemoryBackend{
		forgetFn: func(_ context.Context, _ memory.RetentionPolicy) error {
			return errors.New("semantic backend down")
		},
	}
	s := &Service{semantic: semantic}
	before := time.Now()
	set := RetentionPolicySet{
		Semantic: memory.RetentionPolicy{Before: &before},
	}
	err := s.RunRetention(context.Background(), set)
	if err == nil {
		t.Fatalf("expected error to bubble up from semantic tier")
	}
	if !semantic.called {
		t.Errorf("expected semantic.Forget to have been attempted")
	}
}
