package software

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capCoverage measures the statement coverage of params.packages by running
// go test -coverprofile in params.workdir (docs/roadmap.md, Phase 41 step 10).
//
// The file is coverage_verifier.go, not coverage.go, because .gitignore's
// "coverage.*" (meant for cover profiles) would leave coverage.go untracked.
const capCoverage = "software.verify.coverage"

// packagePattern is what one Go package pattern may hold: import path
// characters and the "..." wildcard. No shell metacharacter, no '=', no
// space; a leading '-' is refused separately because go test would read it as
// a flag (-exec runs any program).
var packagePattern = regexp.MustCompile(`^[A-Za-z0-9._~+@/-]+$`)

// coverPercent turns a Go cover profile into its statement-weighted
// percentage. A block listed more than once (go test -coverpkg writes one
// line per block per test binary) counts once, covered if any line says so.
// A profile that measures no statement is an error, never 0 (AGENTS.md rule
// 10: blind, not quiet).
func coverPercent(profile string) (float64, error) {
	lines := strings.Split(strings.TrimSpace(profile), "\n")
	if !strings.HasPrefix(lines[0], "mode: ") {
		return 0, errors.New("cover profile has no mode line")
	}
	type block struct {
		statements int
		covered    bool
	}
	blocks := map[string]block{}
	for i, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, fmt.Errorf("cover profile line %d is not 'file:range statements count': %q", i+2, line)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil || statements < 0 {
			return 0, fmt.Errorf("cover profile line %d has statement count %q", i+2, fields[1])
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil || count < 0 {
			return 0, fmt.Errorf("cover profile line %d has hit count %q", i+2, fields[2])
		}
		b := blocks[fields[0]]
		b.statements = statements
		b.covered = b.covered || count > 0
		blocks[fields[0]] = b
	}
	total, covered := 0, 0
	for _, b := range blocks {
		total += b.statements
		if b.covered {
			covered += b.statements
		}
	}
	if total == 0 {
		return 0, errors.New("cover profile measures no statement")
	}
	return 100 * float64(covered) / float64(total), nil
}

// packagePatterns splits params.packages into the Go package patterns go test
// is given, refusing anything that is not one.
func packagePatterns(packages string) ([]string, error) {
	patterns := strings.Fields(packages)
	if len(patterns) == 0 {
		return nil, errors.New("params.packages is empty: name the Go package patterns to measure, e.g. './...'")
	}
	for _, p := range patterns {
		if strings.HasPrefix(p, "-") || !packagePattern.MatchString(p) {
			return nil, fmt.Errorf("params.packages holds %q, which is not a Go package pattern", p)
		}
	}
	return patterns, nil
}

// coverage answers software.verify.coverage: it runs go test with a cover
// profile over the validated patterns, without a shell, and returns the
// statement-weighted percentage as one first-party Measurement. A run that
// cannot measure is a failed result carrying stderr and no Measurement.
func (e *verifyEnv) coverage(ctx context.Context, a environment.Action) environment.ActionResult {
	patterns, err := packagePatterns(asString(a.Params, "packages"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil)
	}
	workdir, err := e.shell.resolveWorkdir(asString(a.Params, "workdir"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil)
	}
	dir, err := os.MkdirTemp("", "karakuri-cover-")
	if err != nil {
		return failureResult(e.id, a.CapabilityID, "create the cover profile directory: "+err.Error(), nil)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	profilePath := filepath.Join(dir, "cover.out")
	f, err := os.OpenFile(profilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- the path is inside the temp dir this function created
	if err != nil {
		return failureResult(e.id, a.CapabilityID, "create the cover profile: "+err.Error(), nil)
	}
	_ = f.Close()

	cmdCtx, cancel := context.WithTimeout(ctx, e.shell.timeout)
	defer cancel()
	args := append([]string{"test", "-count=1", "-coverprofile=" + profilePath}, patterns...)
	c := exec.CommandContext(cmdCtx, "go", args...) // #nosec G204 -- every pattern passed packagePatterns, the profile path is our own temp file, and no shell is involved
	c.Dir = workdir
	var stderr bytes.Buffer
	c.Stderr = &stderr
	source := "go test -count=1 -coverprofile " + strings.Join(patterns, " ")
	extra := map[string]any{"cmd": source, "workdir": workdir, "verifier": string(a.CapabilityID)}
	if err := c.Run(); err != nil {
		extra["stderr"] = truncate(stderr.String(), 4000)
		return failureResult(e.id, a.CapabilityID, fmt.Sprintf("%s failed: %v: %s", source, err, truncate(stderr.String(), 1000)), extra)
	}
	profile, err := os.ReadFile(profilePath) // #nosec G304 -- the path is inside the temp dir this function created
	if err != nil {
		return failureResult(e.id, a.CapabilityID, "read the cover profile: "+err.Error(), extra)
	}
	percent, err := coverPercent(string(profile))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), extra)
	}

	res := environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"action":   string(a.CapabilityID),
			"status":   "ok",
			"env_id":   string(e.id),
			"verifier": string(a.CapabilityID),
			"cmd":      source,
			"workdir":  workdir,
		},
		Measurements: []environment.Measurement{{
			Name: "coverage", Value: percent, Unit: "percent",
			Source: source, Trust: environment.TrustOperator,
		}},
	}
	if files, trust, ok := e.changedFiles(ctx, a.Params); ok {
		res.StateDelta["changed_files"] = files
		if trust.IsThirdParty() {
			res.Trust = trust
		}
	}
	return res
}
