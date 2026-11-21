package reconcile

// Phase 40 step 9a: the production reader of a cut-off pass's branch.
//
// Read before writing these (2026-11-21): the loop persists a plan only as the
// "plan" of a learn-step LoopIteration (feature/loop/learn.go), whose actions
// are {capability, params, ...}. The branch is params.branch of a write
// ("act") action and its base params.base_branch. The forge is the version
// control instance bound to the twin (domains/software boundVC); an empty repo
// is the adapter's default.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// recordingForge answers CompareCommits and records what it was asked.
type recordingForge struct {
	versioncontrol.VersionControlAdapter
	commits []versioncontrol.Commit
	err     error
	calls   [][3]string
}

func (r *recordingForge) Name() string { return "recording" }
func (r *recordingForge) Active() bool { return true }
func (r *recordingForge) CompareCommits(_ context.Context, repo, base, head string) ([]versioncontrol.Commit, error) {
	r.calls = append(r.calls, [3]string{repo, base, head})
	return r.commits, r.err
}

type plannedAct struct {
	Capability string         `json:"capability"`
	Params     map[string]any `json:"params"`
}

func act(capability string, params map[string]any) plannedAct {
	return plannedAct{Capability: capability, Params: params}
}

func savePlan(t *testing.T, store storage.StorageAdapter, obj objective.ObjectiveID, n int, acts ...plannedAct) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"plan": map[string]any{"actions": acts}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLoopIteration(context.Background(), storage.LoopIteration{
		ID: fmt.Sprintf("li-%s-%d", obj, n), ObjectiveID: string(obj), Number: n,
		Step: "learn", InputJSON: string(in), OutputJSON: "{}", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func readerFor(f *fixture, vc versioncontrol.VersionControlAdapter) BranchCommitsFunc {
	return NewBranchCommitsReader(f.store, func(context.Context, objective.Objective) versioncontrol.VersionControlAdapter { return vc })
}

var streamObj = objective.Objective{ID: "obj-1", TwinID: "twin-1"}

func TestBranchCommitsReaderTakesBranchAndBaseFromPlannedWrites(t *testing.T) {
	f := newFixture(t, Config{})
	savePlan(t, f.store, streamObj.ID, 1,
		// A read naming another branch is not the stream's branch.
		act("software.verify.stream_pr", map[string]any{"branch": "karakuri/other"}),
		act("software.act.delegate_to_cli", map[string]any{"branch": "karakuri/phase-40-step-09-quota-order", "base_branch": "develop", "repo": "acme/api"}),
	)
	forge := &recordingForge{commits: []versioncontrol.Commit{{SHA: "aaa111", Message: "feat: first"}, {SHA: "bbb222", Message: "feat: second"}}}

	branch, commits, err := readerFor(f, forge)(context.Background(), streamObj)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	if branch != "karakuri/phase-40-step-09-quota-order" {
		t.Errorf("branch = %q, want the planned write's params.branch", branch)
	}
	want := [][3]string{{"acme/api", "develop", "karakuri/phase-40-step-09-quota-order"}}
	if !reflect.DeepEqual(forge.calls, want) {
		t.Errorf("CompareCommits(repo, base, head) calls = %v, want %v", forge.calls, want)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %v, want the two the forge gave", commits)
	}
}

func TestBranchCommitsReaderBaseDefaultsToMain(t *testing.T) {
	f := newFixture(t, Config{})
	savePlan(t, f.store, streamObj.ID, 1, act("software.act.write_code", map[string]any{"branch": "karakuri/x"}))
	forge := &recordingForge{commits: []versioncontrol.Commit{{SHA: "aaa111"}}}

	if _, _, err := readerFor(f, forge)(context.Background(), streamObj); err != nil {
		t.Fatalf("reader: %v", err)
	}
	want := [][3]string{{"", "main", "karakuri/x"}}
	if !reflect.DeepEqual(forge.calls, want) {
		t.Errorf("calls = %v, want %v", forge.calls, want)
	}
}

func TestBranchCommitsReaderNoBranchInAnyPlanIsAnError(t *testing.T) {
	f := newFixture(t, Config{})
	savePlan(t, f.store, streamObj.ID, 1, act("software.act.send_message", map[string]any{"text": "hi"}))
	forge := &recordingForge{}

	_, commits, err := readerFor(f, forge)(context.Background(), streamObj)
	if err == nil {
		t.Fatalf("no planned action names a branch, got commits %v and no error", commits)
	}
	if len(forge.calls) != 0 {
		t.Errorf("the forge was asked %v with no branch to ask about", forge.calls)
	}
}

func TestBranchCommitsReaderNoPlanAtAllIsAnError(t *testing.T) {
	f := newFixture(t, Config{})
	if _, commits, err := readerFor(f, &recordingForge{})(context.Background(), streamObj); err == nil {
		t.Fatalf("no plan recorded, got commits %v and no error", commits)
	}
}

func TestBranchCommitsReaderForgeErrorIsAnError(t *testing.T) {
	f := newFixture(t, Config{})
	savePlan(t, f.store, streamObj.ID, 1, act("software.act.write_code", map[string]any{"branch": "karakuri/x"}))
	boom := errors.New("forge unavailable")

	_, commits, err := readerFor(f, &recordingForge{err: boom})(context.Background(), streamObj)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the forge's error; commits %v", err, commits)
	}
}

func TestBranchCommitsReaderNoOpAdapterIsAnError(t *testing.T) {
	f := newFixture(t, Config{})
	savePlan(t, f.store, streamObj.ID, 1, act("software.act.write_code", map[string]any{"branch": "karakuri/x"}))

	_, commits, err := readerFor(f, versioncontrol.NewNoOp())(context.Background(), streamObj)
	if !errors.Is(err, versioncontrol.ErrUnsupported) {
		t.Fatalf("err = %v, want one wrapping ErrUnsupported, never an empty list (commits %v)", err, commits)
	}
}
