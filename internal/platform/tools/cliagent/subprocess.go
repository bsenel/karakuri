package cliagent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runStreaming launches a CLI subprocess and forwards each stdout line to
// `onLine`. Stderr is collected separately and returned in the final report.
// Honors in.WorktreePath as cwd, in.Env (merged onto os.Environ()), and
// in.TimeoutSeconds (overrides ctx deadline when > 0).
//
// Returns exit code + accumulated stderr + any execution error. A non-zero
// exit code is NOT returned as an error — callers (loop verify step) treat
// failed CLI runs as data, not adapter errors.
func runStreaming(ctx context.Context, in DelegateInput, name string, args []string, onLine func(string)) (exitCode int, stderr string, err error) {
	if in.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(in.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = in.WorktreePath
	cmd.Env = mergedEnv(in.Env)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return -1, "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return -1, "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return -1, "", fmt.Errorf("start %s: %w", name, err)
	}

	stderrCh := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(stderrPipe)
		stderrCh <- string(buf)
	}()

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4 MiB max line — CLIs can emit large JSON events
	for scanner.Scan() {
		onLine(scanner.Text())
	}

	waitErr := cmd.Wait()
	stderr = <-stderrCh

	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else {
		exitCode = -1
	}

	// We don't propagate non-zero exit as an error — let the loop see the data.
	// Real adapter errors (binary not found, pipe failed) come from Start/pipe above.
	if waitErr != nil && exitCode <= 0 {
		err = waitErr
	}
	return exitCode, stderr, err
}

// mergedEnv overlays the per-call env vars onto the process env.
func mergedEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	have := map[string]int{}
	out := append([]string{}, os.Environ()...)
	for i, e := range out {
		if eq := strings.IndexByte(e, '='); eq > 0 {
			have[e[:eq]] = i
		}
	}
	for k, v := range extra {
		entry := k + "=" + v
		if i, ok := have[k]; ok {
			out[i] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}

// binaryAvailable returns true if a binary is found on PATH.
func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
