package software

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13: the minimum time between empty-pipeline cycles is counted
// from the end of the last cycle of either trigger. A cycle's end is the
// run_end audit row of an objective made from the discovery template.

var runEndSeq int

func saveRunEnd(t *testing.T, s storage.StorageAdapter, objective, template, kind string, at time.Time) {
	t.Helper()
	runEndSeq++
	err := s.SaveToolEvent(context.Background(), storage.ToolEvent{
		ID: fmt.Sprintf("run-end-test-%d", runEndSeq), ObjectiveID: objective, Kind: kind,
		TemplateID: template, Success: true, PayloadJSON: "{}", CreatedAt: at,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestLastCycleEnd_IsTheNewestRunEndOfAnyDiscoveryObjective(t *testing.T) {
	s := newPostedStore(t)
	old := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Second)
	newest := time.Now().UTC().Add(-5 * time.Hour).Truncate(time.Second)
	saveRunEnd(t, s, "obj-scheduled", marketDiscoveryTemplateID, storage.ToolEventRunEnd, old)
	// A second objective, as the empty-pipeline trigger would start.
	saveRunEnd(t, s, "obj-empty-pipeline", marketDiscoveryTemplateID, storage.ToolEventRunEnd, newest)
	// Noise: newer rows that are not this stream's cycle end.
	later := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	saveRunEnd(t, s, "obj-delivery", roadmapDeliveryTemplateID, storage.ToolEventRunEnd, later)
	saveRunEnd(t, s, "obj-scheduled", marketDiscoveryTemplateID, storage.ToolEventExecute, later)

	got, err := newLastCycleEnd(s, marketDiscoveryTemplateID)(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got.Equal(newest) {
		t.Errorf("last cycle end = %v, want %v (the newest discovery run_end)", got, newest)
	}
}

func TestLastCycleEnd_NoRecordIsZeroNotAnError(t *testing.T) {
	s := newPostedStore(t)
	saveRunEnd(t, s, "obj-delivery", roadmapDeliveryTemplateID, storage.ToolEventRunEnd, time.Now().UTC())

	got, err := newLastCycleEnd(s, marketDiscoveryTemplateID)(context.Background())
	if err != nil {
		t.Fatalf("no discovery cycle yet is a record of none, got %v", err)
	}
	if !got.IsZero() {
		t.Errorf("last cycle end = %v, want zero", got)
	}
}

// failingLog is a log that cannot be read.
type failingLog struct{ storage.StorageAdapter }

func (failingLog) ListToolEvents(context.Context, storage.ToolEventFilter) ([]storage.ToolEvent, error) {
	return nil, errors.New("database is locked")
}

func TestLastCycleEnd_AnUnreadableRecordIsAnErrorNotZero(t *testing.T) {
	got, err := newLastCycleEnd(failingLog{}, marketDiscoveryTemplateID)(context.Background())
	if err == nil {
		t.Fatalf("an unreadable record returned %v and no error; zero would read as no cycle ever", got)
	}
}

// buildDiscoveryTrigger builds the registered environment from a pack that
// has (or lacks) the audit log, over a forge whose roadmap has nothing Planned.
func buildDiscoveryTrigger(t *testing.T, s storage.StorageAdapter) environment.Environment {
	t.Helper()
	forge := &roadmapForge{fakeForge: &fakeForge{}, text: roadmapNothingPlanned}
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", versioncontrol.VersionControlAdapter(forge))
	pack := New()
	pack.tools = reg
	if s != nil {
		pack.WithStorage(s)
	}
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != "software.env.discovery_trigger" {
			continue
		}
		env, err := fac.Build(environment.BuildContext{AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}
	t.Fatal("no discovery_trigger factory")
	return nil
}

// With the record wired the environment is no longer blind for the
// last-cycle reason: a recent cycle holds it back (has_work false), an old
// one or none lets the empty pipeline start a cycle.
func TestRegisteredDiscoveryTriggerReadsTheLastCycleFromTheRunRecords(t *testing.T) {
	cases := map[string]struct {
		events   func(t *testing.T, s storage.StorageAdapter)
		wantWork bool
	}{
		"a cycle ended inside the minimum time": {func(t *testing.T, s storage.StorageAdapter) {
			saveRunEnd(t, s, "obj-1", marketDiscoveryTemplateID, storage.ToolEventRunEnd, time.Now().UTC().Add(-time.Hour))
		}, false},
		"the last cycle ended long ago": {func(t *testing.T, s storage.StorageAdapter) {
			saveRunEnd(t, s, "obj-1", marketDiscoveryTemplateID, storage.ToolEventRunEnd, time.Now().UTC().Add(-DiscoveryMinGap-48*time.Hour))
		}, true},
		"no cycle yet": {func(*testing.T, storage.StorageAdapter) {}, true},
		"only another stream ran recently": {func(t *testing.T, s storage.StorageAdapter) {
			saveRunEnd(t, s, "obj-2", roadmapDeliveryTemplateID, storage.ToolEventRunEnd, time.Now().UTC().Add(-time.Minute))
		}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newPostedStore(t)
			tc.events(t, s)
			env := buildDiscoveryTrigger(t, s)

			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatalf("Observe is still blind: %v", err)
			}
			if got, _ := obs.State["has_work"].(bool); got != tc.wantWork {
				t.Errorf("has_work = %v, want %v; state %v", got, tc.wantWork, obs.State)
			}
			if snapshotOf(t, env) == "" {
				t.Error("empty SHA although main, the pull requests and the record were readable")
			}
		})
	}
}

// Without the audit log there is no record to read: blind, never an empty pipeline.
func TestRegisteredDiscoveryTriggerWithoutTheRecordIsBlind(t *testing.T) {
	env := buildDiscoveryTrigger(t, nil)
	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
		t.Error("Observe returned no error with no record of the last cycle wired")
	}
	if sha := snapshotOf(t, env); sha != "" {
		t.Errorf("SHA = %q while blind, want empty", sha)
	}
}

// A log that cannot be read leaves the registered environment blind too.
func TestRegisteredDiscoveryTriggerIsBlindWhenTheRecordCannotBeRead(t *testing.T) {
	env := buildDiscoveryTrigger(t, failingLog{})
	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
		t.Error("Observe returned no error although the run records could not be read")
	}
	if sha := snapshotOf(t, env); sha != "" {
		t.Errorf("SHA = %q while blind, want empty", sha)
	}
}
