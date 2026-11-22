package software

import (
	"context"
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 14a: the delivery_work snapshot also says which of the
// delivery stream's pull requests cannot be merged (step 6a), so reconcile can
// show them as blockers. It is shown only: has_work, the reasons and the
// version hash are what they were before, and a read that failed is blind.

const (
	deliveryBranch1 = "karakuri/phase-40-step-14a-x"
	deliveryBranch2 = "karakuri/phase-39-step-3-y"
	uxBranch        = "karakuri/ux-2026-11-20"
)

func unmergeableEntry(branch, stream string) map[string]any {
	return map[string]any{"pr": "1", "branch": branch, "stream": stream, "reason": "conflicts with main"}
}

func TestComputeDeliveryWorkListsOnlyTheDeliveryStreamsUnmergeableBranches(t *testing.T) {
	state := map[string]any{"unmergeable_prs": []map[string]any{
		unmergeableEntry(deliveryBranch1, roadmapDeliveryTemplateID),
		unmergeableEntry(uxBranch, uxImprovementTemplateID),
		unmergeableEntry(deliveryBranch2, roadmapDeliveryTemplateID),
	}}
	got, err := ComputeDeliveryWork(roadmapNothingPlanned, nil, nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{deliveryBranch2, deliveryBranch1}; !reflect.DeepEqual(got.Unmergeable, want) {
		t.Errorf("Unmergeable = %v, want the delivery stream's branches only, sorted: %v", got.Unmergeable, want)
	}

	// Unchanged: step 14a changes no behaviour.
	plain, err := ComputeDeliveryWork(roadmapNothingPlanned, nil, nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.HasWork != plain.HasWork || !reflect.DeepEqual(got.Reasons, plain.Reasons) {
		t.Errorf("verdict %v %v with unmergeable pull requests, %v %v without: they are shown, not work", got.HasWork, got.Reasons, plain.HasWork, plain.Reasons)
	}
	if len(plain.Unmergeable) != 0 {
		t.Errorf("Unmergeable = %v with none listed, want none", plain.Unmergeable)
	}
}

func TestAnUnmergeableReadThatFailedIsBlind(t *testing.T) {
	_, err := ComputeDeliveryWork(roadmapNothingPlanned, nil, nil, map[string]any{"unmergeable_prs_error": "502 from the forge"})
	if err == nil {
		t.Error("no error with unmergeable_prs_error set; an empty list would read as nothing being broken")
	}
	// A forge that has no such signal has no such list: not blind.
	_, err = ComputeDeliveryWork(roadmapNothingPlanned, nil, nil, map[string]any{"unmergeable_prs_error": versioncontrol.ErrUnsupported.Error()})
	if err != nil {
		t.Errorf("ErrUnsupported made the reading blind: %v", err)
	}
}

// deliveryForge has one conflicting pull request on each of the delivery and
// the UX stream's branches, and one clean one on the delivery stream.
func deliveryForge(conflicting ...string) *unmergeableForge {
	f := &unmergeableForge{mergeStates: map[string]string{}}
	f.prs = []versioncontrol.PRSummary{
		openPR(1, deliveryBranch1, "sha1", 0),
		openPR(2, uxBranch, "sha2", 0),
		openPR(3, deliveryBranch2, "sha3", 0),
	}
	for _, id := range conflicting {
		f.mergeStates[id] = "conflicting"
	}
	return f
}

func deliverySnapshot(t *testing.T, forge *unmergeableForge) environment.EnvironmentSnapshot {
	t.Helper()
	main := &mainFile{text: roadmapNothingPlanned}
	env := NewDeliveryWorkEnvironment("software.env.delivery_work", forge, main.read, config.OwnAuthorsConfig{}, nil)
	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.SHA == "" {
		t.Fatal("empty SHA although main and the forge were readable")
	}
	return snap
}

func TestDeliveryWorkSnapshotListsTheDeliveryStreamsUnmergeableBranches(t *testing.T) {
	snap := deliverySnapshot(t, deliveryForge("1", "2", "3"))
	want := []string{deliveryBranch2, deliveryBranch1}
	if got, _ := snap.State["unmergeable_prs"].([]string); !reflect.DeepEqual(got, want) {
		t.Errorf("state[unmergeable_prs] = %#v, want the delivery stream's branches as a sorted []string: %v", snap.State["unmergeable_prs"], want)
	}

	// Shown, not work: has_work, the reasons and the hash are those of a
	// forge whose pull requests all merge.
	clean := deliverySnapshot(t, deliveryForge())
	if snap.State["has_work"] != clean.State["has_work"] || !reflect.DeepEqual(snap.State["reasons"], clean.State["reasons"]) {
		t.Errorf("has_work/reasons moved with unmergeable pull requests: %v %v vs %v %v",
			snap.State["has_work"], snap.State["reasons"], clean.State["has_work"], clean.State["reasons"])
	}
	if snap.SHA != clean.SHA {
		t.Errorf("snapshot SHA moved with unmergeable pull requests (%s vs %s); step 14a changes no behaviour", snap.SHA, clean.SHA)
	}
	if got, _ := clean.State["unmergeable_prs"].([]string); len(got) != 0 {
		t.Errorf("state[unmergeable_prs] = %v for pull requests that merge, want none", got)
	}
}
