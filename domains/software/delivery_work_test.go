package software

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13, delivery half: whether delivery has work is computed from
// main's roadmap and the open pull requests, without a model. An item is
// matched to its pull request by the phase it names in the title suffix
// '(phase N step M)' or by its backlog ID ('EB-014').

const (
	roadmapNothingPlanned = "## Phase 39 — Done thing\n\nbody\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-001 | a | e | p | S | Done |\n"
	roadmapPlannedPhase   = "## Phase 40 — Streams (Planned)\n\nbody\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-001 | a | e | p | S | Done |\n"
	roadmapPlannedEB      = "## Phase 39 — Done thing\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-014 | b | e | p | S | Planned |\n"
)

func ownerComment() []map[string]any {
	return []map[string]any{{"number": 7, "branch": "karakuri/phase-40-step-12-x", "from": "owner", "comment_id": int64(1)}}
}

func TestComputeDeliveryWork(t *testing.T) {
	openFor := func(title string) []versioncontrol.PRSummary {
		return []versioncontrol.PRSummary{{ID: "9", Number: 9, Title: title, HeadBranch: "karakuri/x", OwnAuthor: true}}
	}
	cases := []struct {
		name        string
		roadmap     string
		roadmapErr  error
		prs         []versioncontrol.PRSummary
		state       map[string]any
		wantWork    bool
		wantErr     bool
		wantReasons string // substring of the joined reasons, "" for none required
	}{
		{name: "planned phase, no pull request", roadmap: roadmapPlannedPhase, wantWork: true, wantReasons: "40"},
		{name: "planned backlog entry, no pull request", roadmap: roadmapPlannedEB, wantWork: true, wantReasons: "EB-014"},
		{name: "nothing planned", roadmap: roadmapNothingPlanned, wantWork: false},
		{name: "planned phase whose pull request is open and waiting for the owner",
			roadmap: roadmapPlannedPhase, prs: openFor("feat(x): a thing (phase 40 step 13)"), wantWork: false, wantReasons: "waiting"},
		{name: "planned backlog entry whose pull request is open",
			roadmap: roadmapPlannedEB, prs: openFor("fix(x): a thing (EB-014)"), wantWork: false, wantReasons: "waiting"},
		{name: "a pull request for another phase does not cover the item",
			roadmap: roadmapPlannedPhase, prs: openFor("feat(x): other (phase 41 step 1)"), wantWork: true, wantReasons: "40"},
		{name: "branch with no pull request, nothing planned", roadmap: roadmapNothingPlanned,
			state:    map[string]any{"branches_without_pr": []map[string]any{{"branch": "karakuri/phase-40-step-06-x", "stream": "delivery"}}},
			wantWork: true, wantReasons: "karakuri/phase-40-step-06-x"},
		{name: "owner's unanswered review comment, nothing planned", roadmap: roadmapNothingPlanned,
			state: map[string]any{"unanswered_review_comments": ownerComment()}, wantWork: true, wantReasons: "review comment"},
		{name: "empty finding lists are not work", roadmap: roadmapNothingPlanned,
			state: map[string]any{"branches_without_pr": []map[string]any{}, "unanswered_review_comments": []map[string]any{}}, wantWork: false},
		{name: "main unreadable is blind, not no work", roadmapErr: errors.New("403 from the forge"), wantErr: true},
		{name: "main unreadable is blind even with a finding in hand", roadmapErr: errors.New("boom"),
			state: map[string]any{"unanswered_review_comments": ownerComment()}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeDeliveryWork(tc.roadmap, tc.roadmapErr, tc.prs, tc.state)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("no error although main could not be read; %+v would read as a fact, and blind must never read as 'no work'", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.HasWork != tc.wantWork {
				t.Errorf("HasWork = %v, want %v (reasons %v)", got.HasWork, tc.wantWork, got.Reasons)
			}
			joined := strings.ToLower(strings.Join(got.Reasons, " | "))
			if tc.wantReasons != "" && !strings.Contains(joined, strings.ToLower(tc.wantReasons)) {
				t.Errorf("reasons %q do not name %q; a verdict must say why", joined, tc.wantReasons)
			}
		})
	}
}

// ── the snapshot the delivery stream's reconcile reads ──────────────────────

type mainFile struct {
	text string
	err  error
}

func (m *mainFile) read(context.Context) (string, error) { return m.text, m.err }

func snapshotOf(t *testing.T, env environment.Environment) string {
	t.Helper()
	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return snap.SHA
}

func TestDeliveryWorkSnapshotMovesOnlyWhenTheWorkMoves(t *testing.T) {
	main := &mainFile{text: roadmapNothingPlanned}
	forge := &branchForge{}
	env := NewDeliveryWorkEnvironment("software.env.delivery_work", forge, main.read, config.OwnAuthorsConfig{}, nil)

	idle := snapshotOf(t, env)
	if idle == "" {
		t.Fatal("empty SHA although main was readable; an empty SHA means blind and the supervisor would never sense this stream")
	}
	for i := 0; i < 20; i++ {
		if got := snapshotOf(t, env); got != idle {
			t.Fatalf("tick %d: snapshot %q != %q with nothing changed; every tick would start a pass", i, got, idle)
		}
	}

	// A commit on main that changes no item's state is not a reason to run.
	main.text = roadmapNothingPlanned + "\nA sentence added to a finished phase.\n"
	if got := snapshotOf(t, env); got != idle {
		t.Errorf("snapshot moved on a roadmap edit that adds no Planned item; the stream would plan against nothing")
	}

	// A merge to main that adds a Planned item is what wakes it.
	main.text = roadmapPlannedPhase
	planned := snapshotOf(t, env)
	if planned == idle {
		t.Error("snapshot did not change when main gained a Planned item; the supervisor would sleep through the work")
	}

	// The pass opens a pull request for it: the state changes again (waiting
	// for the owner), so the next tick can settle on it.
	forge.prs = []versioncontrol.PRSummary{{ID: "9", Number: 9, Title: "feat(x): y (phase 40 step 13)", HeadBranch: "karakuri/x", OwnAuthor: true}}
	waiting := snapshotOf(t, env)
	if waiting == planned {
		t.Error("snapshot did not change when the item's pull request opened")
	}
	if waiting == idle {
		t.Error("an item waiting for the owner hashes like an empty pipeline; the two are different states")
	}
}

func TestDeliveryWorkSnapshotIsBlindWhenMainCannotBeRead(t *testing.T) {
	main := &mainFile{err: errors.New("401 bad credentials")}
	env := NewDeliveryWorkEnvironment("software.env.delivery_work", &branchForge{}, main.read, config.OwnAuthorsConfig{}, nil)

	if sha := snapshotOf(t, env); sha != "" {
		t.Errorf("SHA = %q with main unreadable; blind must be an empty SHA (AGENTS.md rule 10)", sha)
	}
	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
		t.Error("Observe returned no error with main unreadable; blind must say so, never look like no work")
	}
}
