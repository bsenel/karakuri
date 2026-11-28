package gateway

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Phase 39 step 4, slice S2: the core forgets workspaces. A worktree is the
// software pack's idea of a workspace and nobody else's, so the capability
// type no longer says a tool needs one, and the orchestrator's wiring holds
// no WorktreeManager to provision it with. This test fails, naming file:line,
// wherever a non-test Go file under the core's trees still names either.
//
// The search is on the text, comments included: a comment that says the loop
// provisions a worktree describes code that no longer exists.

// noWorkspaceTrees are the orchestrator's trees: entry, delivery, use cases
// and domain, and the conformance suite that holds packs to the core's
// contract. Packs (domains/) and adapters (internal/platform/) keep their own
// worktrees and are not searched.
var noWorkspaceTrees = []string{"cmd", "internal/api", "internal/core", "internal/feature", "internal/conformance"}

var workspaceIdent = regexp.MustCompile(`\b(NeedsWorkspace|GrantsWorkspace|WorktreeManager)\b`)

func TestTheCoreNamesNoWorkspace(t *testing.T) {
	root := moduleRoot(t)
	var hits []string
	for _, tree := range noWorkspaceTrees {
		err := filepath.WalkDir(filepath.Join(root, tree), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			rel, _ := filepath.Rel(root, path)
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for line := 1; sc.Scan(); line++ {
				if m := workspaceIdent.FindString(sc.Text()); m != "" {
					hits = append(hits, rel+":"+strconv.Itoa(line)+": "+m)
				}
			}
			return sc.Err()
		})
		if err != nil {
			t.Fatalf("walk %s: %v", tree, err)
		}
	}
	for _, h := range hits {
		t.Errorf("%s; the core does not provision workspaces, the pack that serves a file-writing tool keeps its own", h)
	}
}
