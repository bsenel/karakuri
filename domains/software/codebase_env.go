package software

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// codebaseEnv reads the repository as evidence.
//
// Phase 22 shipped self_improve with a hard `evidence-first` constraint and a
// criterion reading "the proposal names the telemetry that says the problem is
// real". On a fresh deployment the telemetry snapshot is empty: no objectives,
// no reconcile outcomes, no audit rows, no spend. The pack had nothing to say
// and the constraint could not be satisfied — **the deployment that most needs
// a roadmap is the one that cannot produce one**, for as long as it takes to
// accumulate the history it needs, which nobody waits through.
//
// The repository is evidence that exists on day one. The roadmap's own
// deferred lists in particular are a backlog somebody already justified in
// prose, and TODO density and missing tests are the two complaints a codebase
// makes about itself without being asked.
//
// This replaces a noopEnv. `software.env.codebase` has been declared since
// Phase 2 as "static analysis: file tree, symbols, dependency graph" and
// served nothing — a seventh instance of a declaration nothing read, found
// while looking for somewhere to put this.
type codebaseEnv struct {
	id   environment.EnvironmentID
	root string
}

func newCodebaseEnv(id environment.EnvironmentID, root string) *codebaseEnv {
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	return &codebaseEnv{id: id, root: root}
}

func (e *codebaseEnv) ID() environment.EnvironmentID { return e.id }
func (e *codebaseEnv) Domain() string                { return "software" }

func (e *codebaseEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	state, err := e.scan()
	if err != nil {
		return environment.Observation{EnvID: e.id, Timestamp: time.Now().UTC()}, err
	}
	return environment.Observation{
		EnvID:     e.id,
		State:     state,
		Version:   stateVersion(state),
		Timestamp: time.Now().UTC(),
	}, nil
}

func (e *codebaseEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	if a.CapabilityID != CapAnalyseRepo {
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("%s reads the repository; %s cannot be executed here", e.id, a.CapabilityID),
		}, nil
	}
	state, err := e.scan()
	if err != nil {
		return environment.ActionResult{Success: false, Error: err.Error()}, nil
	}
	state["capability"] = string(CapAnalyseRepo)
	return environment.ActionResult{Success: true, StateDelta: state}, nil
}

func (e *codebaseEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *codebaseEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, err := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{
		SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp,
	}, err
}

// ── the scan ─────────────────────────────────────────────────────────────────

// Scan limits. A repository read is cheap only while it stays bounded, and
// this runs on every sense tick of a standing objective.
const (
	maxScanFiles   = 20000
	maxDeferred    = 40
	maxHotPackages = 15
)

func (e *codebaseEnv) scan() (map[string]any, error) {
	if e.root == "" {
		return map[string]any{
			"available": false,
			"reason":    "no repository root is configured for this deployment",
			"evidence":  EvidenceNone,
		}, nil
	}
	if _, err := os.Stat(e.root); err != nil {
		// Unreadable is reported, not raised. The pack degrades the way the
		// telemetry environment does when no reader is wired: it says it is
		// blind rather than failing the capability that asked.
		return map[string]any{
			"available": false,
			"reason":    fmt.Sprintf("repository root %q is not readable", e.root),
			"evidence":  EvidenceNone,
		}, nil
	}

	markers, todos, tests := e.walk()
	deferred := e.deferredWork()

	state := map[string]any{
		"available":         true,
		"root":              e.root,
		"packages":          len(todos),
		"deferred_work":     deferred,
		"todo_density":      rankDensity(todos, maxHotPackages),
		"untested_packages": tests,
		"rules":             markers,
	}
	state["evidence"] = repoEvidenceLevel(deferred, todos, tests)
	return state, nil
}

// walk collects the three per-package facts in one pass, because three walks
// over a repository on every sense tick is three times the I/O for the same
// answer.
func (e *codebaseEnv) walk() (rules []string, todos map[string]int, untested []string) {
	todos = map[string]int{}
	sourceFiles := map[string]int{}
	testFiles := map[string]int{}

	seen := 0
	_ = filepath.WalkDir(e.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || seen > maxScanFiles {
			return nil //nolint:nilerr // an unreadable subtree is skipped, not fatal
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "build", ".cache":
				return filepath.SkipDir
			}
			return nil
		}
		seen++

		rel, relErr := filepath.Rel(e.root, path)
		if relErr != nil {
			return nil
		}
		pkg := filepath.Dir(rel)

		switch {
		case d.Name() == "AGENTS.md":
			rules = append(rules, rel)
		case strings.HasSuffix(d.Name(), "_test.go"):
			testFiles[pkg]++
		case strings.HasSuffix(d.Name(), ".go"):
			sourceFiles[pkg]++
			if n := countMarkers(path); n > 0 {
				todos[pkg] += n
			}
		}
		return nil
	})

	// A package with source and no test file at all. Deliberately *not*
	// called coverage: this counts files, and reporting a file ratio under a
	// name that means "proportion of lines executed by tests" would be the
	// same dishonesty this pack keeps finding elsewhere.
	for pkg, n := range sourceFiles {
		if n > 0 && testFiles[pkg] == 0 {
			untested = append(untested, pkg)
		}
	}
	sort.Strings(untested)
	sort.Strings(rules)
	return rules, todos, untested
}

// countMarkers counts TODO and FIXME comments in one file.
func countMarkers(path string) int {
	f, err := os.Open(path) //nolint:gosec // paths come from walking the configured root
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// Only in comments. "TODO" inside a string literal is usually a
		// message about somebody else's TODO, and counting it inflates the
		// density of exactly the packages that handle tasks.
		if i := strings.Index(line, "//"); i >= 0 {
			rest := line[i:]
			if strings.Contains(rest, "TODO") || strings.Contains(rest, "FIXME") {
				n++
			}
		}
	}
	return n
}

// deferredWork pulls the roadmap's own deferred and open items.
//
// The most valuable evidence in the repository and the cheapest to read: each
// line is a piece of work somebody already argued for in prose, which is a
// better starting point than anything derivable from counting files.
func (e *codebaseEnv) deferredWork() []string {
	f, err := os.Open(filepath.Join(e.root, "docs", "roadmap.md")) //nolint:gosec // fixed path under the configured root
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []string
	var phase string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() && len(out) < maxDeferred {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "## Phase ") {
			phase = strings.TrimPrefix(line, "## ")
			continue
		}
		lower := strings.ToLower(line)
		// The phrases this repository actually uses to mark unfinished work.
		if strings.HasPrefix(lower, "**still open") ||
			strings.HasPrefix(lower, "**deferred") ||
			strings.HasPrefix(lower, "**not covered") {
			entry := line
			if phase != "" {
				entry = phase + " — " + line
			}
			out = append(out, entry)
		}
	}
	return out
}

// rankDensity returns the packages with the most markers, ranked.
//
// Pre-ranked for the same reason the telemetry environment pre-ranks
// bottlenecks: a model asked to order these itself will order them slightly
// differently on every run, and "what should I work on" is not a question
// worth re-deriving nondeterministically.
func rankDensity(todos map[string]int, limit int) []map[string]any {
	type entry struct {
		pkg string
		n   int
	}
	all := make([]entry, 0, len(todos))
	for pkg, n := range todos {
		all = append(all, entry{pkg, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].pkg < all[j].pkg
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]map[string]any, 0, len(all))
	for _, e := range all {
		out = append(out, map[string]any{"package": e.pkg, "markers": e.n})
	}
	return out
}

// repoEvidenceLevel grades the repository the way evidenceLevel grades
// telemetry, and on the same threshold, so "adequate" means one thing across
// both sources rather than two.
func repoEvidenceLevel(deferred []string, todos map[string]int, untested []string) string {
	// A deferred item is work somebody already justified in writing. One is
	// enough to propose from, which is the whole point of reading the
	// repository: it does not need to accumulate.
	if len(deferred) > 0 {
		return EvidenceAdequate
	}
	total := len(untested)
	for _, n := range todos {
		total += n
	}
	switch {
	case total >= minPattern:
		return EvidenceAdequate
	case total > 0:
		return EvidenceThin
	default:
		return EvidenceNone
	}
}
