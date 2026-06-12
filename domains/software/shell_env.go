package software

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// shellEnv is the first real executor in the software pack — runs the
// `software.act.shell_exec` capability via /bin/sh inside a sandbox
// directory. Backs the Phase 14 dogfood follow-up after PR #29
// surfaced that the agent was hallucinating env_ids: the agent now sees
// the catalog, but until a real executor exists every action still
// honestly-fails. shellEnv closes that gap with the minimum substrate
// the agent needs to scaffold modules, init files, write tests, etc.
//
// Safety contract:
//   - Commands run inside a configurable working directory (defaults to
//     the server's CWD). Per-action override via params.workdir, but
//     the env refuses to step outside its DefaultRoot when one is set.
//   - A static denylist blocks obviously destructive patterns
//     (`rm -rf /`, `mkfs`, `dd if=`, sudo escalation, `curl|sh`
//     pipelines, fork bombs). Determined adversaries can bypass via
//     obfuscation — the denylist is a guard against accidental harm,
//     not a sandbox.
//   - Per-command timeout (default 60s, override via params.timeout_sec
//     up to 600s).
//
// Operators who want a more aggressive sandbox can replace the env
// with a containerized executor — the wire shape (params + ActionResult)
// is the contract. This implementation is intentionally simple so the
// Phase 14 dogfood can make forward progress.
type shellEnv struct {
	id          environment.EnvironmentID
	defaultRoot string        // workdir confinement; empty = server's CWD, no confinement
	timeout     time.Duration // default per-command timeout
}

func newShellEnv(id environment.EnvironmentID, defaultRoot string, timeout time.Duration) *shellEnv {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &shellEnv{id: id, defaultRoot: defaultRoot, timeout: timeout}
}

func (e *shellEnv) ID() environment.EnvironmentID { return e.id }
func (e *shellEnv) Domain() string                { return "software" }

func (e *shellEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	// Observation reports the env's configuration so the reason step
	// can pick a workdir or timeout knowingly.
	state := map[string]any{
		"adapter":         "shell.exec",
		"default_root":    e.defaultRoot,
		"default_timeout": e.timeout.String(),
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

func (e *shellEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	if string(a.CapabilityID) != "software.act.shell_exec" {
		// shellEnv only knows shell_exec. Other capabilities routed
		// here are the agent's mistake — fail honestly so the audit
		// log catches the routing error.
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("shellEnv does not handle capability %q (only software.act.shell_exec)", a.CapabilityID),
			StateDelta: map[string]any{
				"action": string(a.CapabilityID),
				"status": "wrong_env",
				"env_id": string(e.id),
			},
		}, nil
	}

	cmd := asString(a.Params, "cmd")
	if cmd == "" {
		return failureResult(e.id, a.CapabilityID, "missing required param 'cmd'", nil), nil
	}

	if reason, blocked := isDangerous(cmd); blocked {
		return failureResult(e.id, a.CapabilityID, fmt.Sprintf("command blocked by safety guardrail: %s", reason), map[string]any{
			"cmd_truncated": truncate(cmd, 200),
		}), nil
	}

	workdir, err := e.resolveWorkdir(asString(a.Params, "workdir"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil), nil
	}

	timeout := e.timeout
	if sec := asInt(a.Params, "timeout_sec"); sec > 0 {
		if sec > 600 {
			sec = 600
		}
		timeout = time.Duration(sec) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(cmdCtx, "/bin/sh", "-c", cmd)
	c.Dir = workdir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	runErr := c.Run()
	exitCode := 0
	timedOut := errors.Is(cmdCtx.Err(), context.DeadlineExceeded)
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if !timedOut {
			// Not a normal exit error — likely a startup failure
			// (e.g. /bin/sh not found, permission denied on workdir).
			return failureResult(e.id, a.CapabilityID, runErr.Error(), map[string]any{
				"cmd":     truncate(cmd, 200),
				"workdir": workdir,
			}), nil
		}
	}

	success := exitCode == 0 && !timedOut
	errMsg := ""
	if timedOut {
		errMsg = fmt.Sprintf("command timed out after %s", timeout)
	} else if !success {
		errMsg = fmt.Sprintf("command exited with code %d", exitCode)
	}

	return environment.ActionResult{
		Success: success,
		Error:   errMsg,
		StateDelta: map[string]any{
			"action":    string(a.CapabilityID),
			"status":    statusFor(success, timedOut),
			"env_id":    string(e.id),
			"cmd":       truncate(cmd, 200),
			"workdir":   workdir,
			"exit_code": exitCode,
			"stdout":    truncate(stdout.String(), 4000),
			"stderr":    truncate(stderr.String(), 4000),
			"timed_out": timedOut,
		},
	}, nil
}

func (e *shellEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *shellEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// resolveWorkdir picks the working directory for the command. When
// defaultRoot is set, the chosen workdir must live inside it; the env
// refuses to escape that root (.. or absolute paths outside).
func (e *shellEnv) resolveWorkdir(requested string) (string, error) {
	if requested == "" {
		// No workdir param → use defaultRoot, or empty (= CWD) if not set.
		return e.defaultRoot, nil
	}
	if e.defaultRoot == "" {
		// No confinement → accept whatever was requested. The operator
		// chose not to set a root.
		return requested, nil
	}
	// Reject absolute paths or .. that would escape the root.
	if strings.HasPrefix(requested, "/") {
		// Allow only if the absolute path lives inside defaultRoot.
		if !strings.HasPrefix(requested, e.defaultRoot+"/") && requested != e.defaultRoot {
			return "", fmt.Errorf("workdir %q is outside default_root %q", requested, e.defaultRoot)
		}
		return requested, nil
	}
	if strings.Contains(requested, "..") {
		return "", fmt.Errorf("workdir %q must not contain '..'", requested)
	}
	return e.defaultRoot + "/" + requested, nil
}

// dangerousPatterns blocks obviously destructive commands as a guard
// against accidental harm. Determined adversaries can obfuscate around
// this — it is NOT a security boundary, just a defensive default.
var dangerousPatterns = []struct {
	name string
	rx   *regexp.Regexp
}{
	{"rm -rf root", regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rfR][a-zA-Z]*\s+)+\/(\s|$)`)},
	{"rm -rf home", regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rfR][a-zA-Z]*\s+)+~(\s|$|/)`)},
	{"rm -rf wildcard root", regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rfR][a-zA-Z]*\s+)+/\*`)},
	{"dd to device", regexp.MustCompile(`\bdd\s+.*of=/dev/`)},
	{"mkfs", regexp.MustCompile(`\bmkfs\b`)},
	{"sudo", regexp.MustCompile(`\bsudo\b`)},
	{"su -", regexp.MustCompile(`\bsu\b`)},
	{"chmod world write", regexp.MustCompile(`\bchmod\s+(-R\s+)?[0-7]*[2-7][2-7]`)},
	{"curl pipe sh", regexp.MustCompile(`(curl|wget|fetch)\b[^|]*\|\s*(sh|bash|zsh|sh\b|bash\b)`)},
	{"eval pipe", regexp.MustCompile(`\beval\s+\$\(\s*(curl|wget|fetch)`)},
	{"fork bomb", regexp.MustCompile(`:\s*\(\s*\)\s*{[^}]*:\|:`)},
	{"history clear", regexp.MustCompile(`\bhistory\s+-c\b`)},
}

func isDangerous(cmd string) (string, bool) {
	for _, p := range dangerousPatterns {
		if p.rx.MatchString(cmd) {
			return p.name, true
		}
	}
	return "", false
}

func asInt(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

func statusFor(success, timedOut bool) string {
	switch {
	case success:
		return "ok"
	case timedOut:
		return "timeout"
	default:
		return "exit_nonzero"
	}
}

func failureResult(id environment.EnvironmentID, capID interface{}, msg string, extra map[string]any) environment.ActionResult {
	delta := map[string]any{
		"action": fmt.Sprintf("%v", capID),
		"status": "rejected",
		"env_id": string(id),
	}
	for k, v := range extra {
		delta[k] = v
	}
	return environment.ActionResult{
		Success:    false,
		Error:      msg,
		StateDelta: delta,
	}
}
