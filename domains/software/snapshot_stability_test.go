package software

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/messaging"
	"github.com/bsenel/karakuri/internal/platform/tools/projectmgmt"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// The property the reconcile supervisor rests on: an environment whose world
// has not moved hashes to the same SHA every time it is asked.
//
// It is asserted by *running* each shipped environment rather than by reading
// stateVersion back, because sorting the keys only fixes one of the two ways a
// hash can wobble. A state whose values are built by ranging a map — a ranked
// TODO list, a package table — would still differ run to run with every key in
// order, and no test of stateVersion in isolation would notice.
//
// Repeated rather than paired: Go randomises map iteration per range, so two
// calls agree by luck often enough that a two-shot test passes on a broken
// hash. Twenty is cheap and leaves no room for it.
const stabilityRounds = 20

func assertStableSnapshot(t *testing.T, name string, env environment.Environment) {
	t.Helper()
	ctx := context.Background()

	first, err := env.Snapshot(ctx)
	if err != nil {
		t.Fatalf("%s: snapshot: %v", name, err)
	}
	if first.SHA == "" {
		t.Fatalf("%s: no SHA to compare; use assertNoSnapshotSHA for environments that decline to hash", name)
	}
	for i := 0; i < stabilityRounds; i++ {
		next, err := env.Snapshot(ctx)
		if err != nil {
			t.Fatalf("%s: snapshot %d: %v", name, i, err)
		}
		if next.SHA != first.SHA {
			t.Fatalf("%s: SHA moved without the world moving (%q != %q on round %d); "+
				"every sense tick would read this as drift and run the expensive tier",
				name, next.SHA, first.SHA, i)
		}
	}
}

// Every environment the reconcile supervisor can fingerprint, with adapters
// that return fixed data — so anything that moves is the environment's own
// nondeterminism and nothing else.
//
// Four of the six do the work: gitEnv, cliEnv, codebaseEnv and shellEnv hash
// multi-key state and all four failed before the sort went in. The other two
// are here to guard the future, not to prove the present, and it is worth
// saying so rather than counting them as coverage: ticketEnv and commsEnv reach
// Snapshot through an Observe with no filter, so their state is a single
// adapter name — nothing a map order can permute — and noopEnv returns a
// constant. They start earning their place the day either grows a second key.
func TestShippedEnvironmentsHashStably(t *testing.T) {
	repo := t.TempDir()
	for _, f := range []struct{ path, body string }{
		{"go.mod", "module scratch\n"},
		{"a/a.go", "package a\n\n// TODO: one\nfunc A() {}\n"},
		{"a/a_test.go", "package a\n"},
		{"b/b.go", "package b\n\n// TODO: two\n// TODO: three\nfunc B() {}\n"},
		{"c/c.go", "package c\n\nfunc C() {}\n"},
		{"AGENTS.md", "# rules\n"},
		{"docs/roadmap.md", "## Phase 1 — Something (Planned)\n\nDeferred until later.\n"},
	} {
		p := filepath.Join(repo, f.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	vc := &vcStub{
		commits: []versioncontrol.Commit{
			{SHA: "aaa", Message: "first", Author: "someone", Date: time.Unix(1, 0).UTC()},
			{SHA: "bbb", Message: "second", Author: "another", Date: time.Unix(2, 0).UTC()},
		},
		prs: []versioncontrol.PRSummary{
			{ID: "1", Title: "one", URL: "u1", CheckState: "success"},
			{ID: "2", Title: "two", URL: "u2", CheckState: "failure", FailingChecks: []string{"lint", "test"}},
		},
	}

	for _, tc := range []struct {
		name string
		env  environment.Environment
	}{
		{"gitEnv", &gitEnv{id: EnvGit, vc: vc}},
		{"ticketEnv", &ticketEnv{id: "software.env.ticket", pm: projectmgmt.NewNoOp()}},
		{"commsEnv", &commsEnv{id: "software.env.communication", msg: messaging.NewNoOp()}},
		{"cliEnv", &cliEnv{id: "software.env.cli_agent", cli: cliagent.NewNoOp()}},
		{"codebaseEnv", newCodebaseEnv("software.env.codebase", repo)},
		{"shellEnv", newShellEnv("software.env.shell", repo, time.Second)},
		{"noopEnv", &noopEnv{id: "software.env.ci"}},
	} {
		t.Run(tc.name, func(t *testing.T) { assertStableSnapshot(t, tc.name, tc.env) })
	}
}

// researchEnv deliberately carries no SHA: there is no "current value" of the
// field, and a standing objective must not reconcile because a search engine's
// results moved. Pinned so the fix above does not accidentally give it one.
func TestResearchEnvironmentStillDeclinesToHash(t *testing.T) {
	e := newResearchEnv("software.env.research", &researchStub{
		findings: []research.Finding{{Title: "a paper"}},
	})
	snap, err := e.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.SHA != "" {
		t.Errorf("SHA = %q, want empty — research has no state that drifts", snap.SHA)
	}
}

// The unit underneath, kept because it is the line that was wrong and a
// failure here names the cause rather than the symptom.
func TestStateVersionIsOrderIndependent(t *testing.T) {
	state := map[string]any{
		"adapter": "github", "commits": []string{"a", "b"}, "prs": []string{"p1"},
		"failing_prs": nil, "available": true, "packages": 3, "root": "/x", "evidence": "adequate",
	}
	first := stateVersion(state)
	for i := 0; i < 200; i++ {
		if got := stateVersion(state); got != first {
			t.Fatalf("round %d: %q != %q — stateVersion still depends on map order", i, got, first)
		}
	}

	// And it is still a hash of the content: change one value, get a different
	// SHA. A "fix" that returned a constant would pass everything above.
	state["packages"] = 4
	if stateVersion(state) == first {
		t.Error("stateVersion did not change when the state did")
	}
}
