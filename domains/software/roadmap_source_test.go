package software

import (
	"context"
	"errors"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13: the delivery_work and discovery_trigger environments the
// pack registers read main's roadmap from the forge, through FileAt, rather
// than from a stub that is always blind.

const roadmapWithPlannedPhase = "## Phase 40 — Streams (Planned)\n\nbody\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-001 | a | e | p | S | Done |\n"

// roadmapForge answers FileAt from a canned file and records what it was asked.
type roadmapForge struct {
	*fakeForge
	text    string
	readErr error

	gotRepo, gotRef, gotPath string
	reads                    int
}

// ListClosedPRs answers with no closed pull request, so the discovery trigger
// built over this forge reads an empty closing comment instead of going blind.
func (f *roadmapForge) ListClosedPRs(context.Context, string, int) ([]versioncontrol.PRSummary, error) {
	return nil, nil
}

func (f *roadmapForge) FileAt(_ context.Context, repo, ref, path string) (string, error) {
	f.reads++
	f.gotRepo, f.gotRef, f.gotPath = repo, ref, path
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.text, nil
}

func (roadmapForge) Active() bool { return true }

// buildRegistered builds the environment the pack registers under id, bound to forge.
func buildRegistered(t *testing.T, id environment.EnvironmentID, forge versioncontrol.VersionControlAdapter) environment.Environment {
	t.Helper()
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", forge)
	pack := New()
	pack.tools = reg
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != id {
			continue
		}
		env, err := fac.Build(environment.BuildContext{AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build %s: %v", id, err)
		}
		return env
	}
	t.Fatalf("no %s environment factory", id)
	return nil
}

func TestRegisteredDeliveryWorkReadsMainsRoadmapFromTheForge(t *testing.T) {
	forge := &roadmapForge{fakeForge: &fakeForge{}, text: roadmapWithPlannedPhase}
	env := buildRegistered(t, "software.env.delivery_work", forge)

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.State["has_work"] != true {
		t.Errorf("has_work = %v, want true for a Planned phase with no open pull request; state %v", obs.State["has_work"], obs.State)
	}
	if forge.gotRef != "main" || forge.gotPath != "docs/roadmap.md" {
		t.Errorf("read %q at %q, want docs/roadmap.md at main", forge.gotPath, forge.gotRef)
	}
	snap, err := env.Snapshot(context.Background())
	if err != nil || snap.SHA == "" {
		t.Errorf("Snapshot = %+v, %v; want a SHA, the forge is readable", snap, err)
	}
}

func TestRegisteredDeliveryWorkIsBlindWhenTheForgeCannotReadTheRoadmap(t *testing.T) {
	forge := &roadmapForge{fakeForge: &fakeForge{}, readErr: errors.New("forge unreachable")}
	env := buildRegistered(t, "software.env.delivery_work", forge)

	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
		t.Error("Observe: want an error when the roadmap read fails")
	}
	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.SHA != "" {
		t.Errorf("Snapshot SHA = %q, want empty (blind)", snap.SHA)
	}
	if forge.reads == 0 {
		t.Error("the forge was never asked for the roadmap, so the failure is the stub's")
	}
}

// The discovery trigger reads the same roadmap; its other sources (the last
// cycle) are a later slice, so here only the roadmap read is pinned: the
// forge must be asked for it, whatever the verdict.
func TestRegisteredDiscoveryTriggerReadsMainsRoadmapFromTheForge(t *testing.T) {
	forge := &roadmapForge{fakeForge: &fakeForge{}, text: roadmapWithPlannedPhase}
	env := buildRegistered(t, "software.env.discovery_trigger", forge)

	_, _ = env.Observe(context.Background(), environment.ObservationQuery{})
	if forge.reads == 0 || forge.gotRef != "main" || forge.gotPath != "docs/roadmap.md" {
		t.Errorf("forge asked %d times for %q at %q, want docs/roadmap.md at main", forge.reads, forge.gotPath, forge.gotRef)
	}
}
