package software

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capCLIStartup measures how long a CLI takes to start: the wall time from
// starting params.binary, a path relative to params.workdir, with params.args
// (default --help) to its exit, in ms (docs/roadmap.md, Phase 41 step 11).
const capCLIStartup = "software.verify.cli_startup_ms"

// cliStartupTimeout bounds one run. A CLI that has not exited by then has no
// startup time; the run is killed and the result fails.
const cliStartupTimeout = 30 * time.Second

// startupBinary resolves binary, a path relative to workdir, to a regular
// file inside workdir. The lookup goes through an os.Root, so an absolute
// path, a ".." that climbs out, or a symlink anywhere on the path that points
// outside workdir is refused by the kernel-side lookup rather than by string
// checks. Every error names params.binary.
func startupBinary(workdir, binary string) (string, error) {
	if binary == "" {
		return "", errors.New("params.binary is empty: name the CLI to time, relative to params.workdir, e.g. 'bin/krk'")
	}
	if filepath.IsAbs(binary) {
		return "", fmt.Errorf("params.binary %q is absolute; give it relative to params.workdir", binary)
	}
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return "", fmt.Errorf("open params.workdir to time params.binary %q: %w", binary, err)
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(binary)
	if err != nil {
		return "", fmt.Errorf("params.binary %q is not a file inside params.workdir: %w", binary, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("params.binary %q is not a regular file", binary)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("params.binary %q is not executable", binary)
	}
	return filepath.Join(workdir, binary), nil
}

// startupArgs reads params.args, a list of strings, and defaults to --help.
// A plan's JSON gives []any; a Go caller may give []string.
func startupArgs(params map[string]any) ([]string, error) {
	switch v := params["args"].(type) {
	case nil:
		return []string{"--help"}, nil
	case []string:
		return v, nil
	case []any:
		args := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("params.args[%d] for params.binary is %T, want a string", i, item)
			}
			args = append(args, s)
		}
		return args, nil
	default:
		return nil, fmt.Errorf("params.args for params.binary is %T, want a list of strings", v)
	}
}

// cliStartupMs answers software.verify.cli_startup_ms. It runs params.binary
// once, without a shell, and reports the wall time from its start to its
// exit: one action is one sample, and a criterion that wants the median of
// several plans several actions. A run it cannot start, that exits non-zero,
// or that does not exit within cliStartupTimeout is a failed result with no
// Measurement, never a 0 that would read as the fastest start there is
// (AGENTS.md rule 10).
func (e *verifyEnv) cliStartupMs(ctx context.Context, a environment.Action) environment.ActionResult {
	workdir, err := e.shell.resolveWorkdir(asString(a.Params, "workdir"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, "params.binary cannot be timed: "+err.Error(), nil)
	}
	if workdir == "" {
		workdir = "."
	}
	extra := map[string]any{"workdir": workdir, "verifier": string(a.CapabilityID)}
	binary := asString(a.Params, "binary")
	path, err := startupBinary(workdir, binary)
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), extra)
	}
	args, err := startupArgs(a.Params)
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), extra)
	}

	runCtx, cancel := context.WithTimeout(ctx, cliStartupTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, args...) // #nosec G204 -- a regular file resolved inside params.workdir through os.Root; no shell
	cmd.Dir = workdir
	cmd.WaitDelay = 5 * time.Second
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	if runCtx.Err() != nil {
		return failureResult(e.id, a.CapabilityID,
			fmt.Sprintf("params.binary %q did not exit within %s: no startup time", binary, cliStartupTimeout), extra)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return failureResult(e.id, a.CapabilityID,
				fmt.Sprintf("params.binary %q exited with status %d: no startup time", binary, exitErr.ExitCode()), extra)
		}
		return failureResult(e.id, a.CapabilityID, fmt.Sprintf("run params.binary %q: %v", binary, err), extra)
	}

	ms := float64(elapsed.Microseconds()) / 1000
	return environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"action":   string(a.CapabilityID),
			"status":   "ok",
			"env_id":   string(e.id),
			"verifier": string(a.CapabilityID),
			"binary":   binary,
			"workdir":  workdir,
		},
		Measurements: []environment.Measurement{{
			Name: "cli_startup_ms", Value: ms, Unit: "ms",
			Source: "wall time of " + filepath.Join(workdir, binary), Trust: environment.TrustOperator,
		}},
	}
}
