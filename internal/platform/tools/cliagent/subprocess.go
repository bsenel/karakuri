package cliagent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/bsenel/karakuri/internal/platform/procenv"
)

// stopGrace is the stop bound (Phase 37 step 2): from the moment a run's
// context is cancelled, its process group is asked to end, killed if it has
// not ended by the last tenth of the bound, and the run returns when the group
// is gone or the bound is up, whichever comes first.
const stopGrace = 10 * time.Second

// stopBound is the bound runStreaming keeps to on a cancelled run; tests
// shorten it.
var stopBound = stopGrace

// stopPoll is how often a stop looks whether the group has gone.
const stopPoll = 20 * time.Millisecond

// cutError marks a run that a cancelled context ended before it finished.
// groupExited says whether the process group was seen to exit by the time
// runStreaming returned.
type cutError struct {
	groupExited bool
	cause       error
}

func (e *cutError) Error() string {
	if e.groupExited {
		return fmt.Sprintf("run cut before it finished (%v); the process group exited", e.cause)
	}
	return fmt.Sprintf("run cut before it finished (%v); the process group was not seen to exit", e.cause)
}

func (e *cutError) Unwrap() error { return e.cause }

// RunCut reports whether err says a delegated run was cut by its context
// (a stop or a timeout), and if so whether its process group was seen to exit.
func RunCut(err error) (cut, groupExited bool) {
	var c *cutError
	if errors.As(err, &c) {
		return true, c.groupExited
	}
	return false, false
}

// runStreaming launches a CLI subprocess and forwards each stdout line to
// `onLine`. Stderr is collected separately and returned in the final report.
// Honors in.WorktreePath as cwd, in.Env (merged onto os.Environ()), and
// in.TimeoutSeconds (overrides ctx deadline when > 0).
//
// Returns exit code + accumulated stderr + any execution error. A non-zero
// exit code is NOT returned as an error — callers (loop verify step) treat
// failed CLI runs as data, not adapter errors. A run whose context is
// cancelled is stopped within stopBound (see stopGroup) and returns a
// *cutError.
func runStreaming(ctx context.Context, in DelegateInput, name string, args []string, onLine func(string)) (exitCode int, stderr string, err error) {
	if in.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(in.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// exec.Command, not CommandContext: a cancelled run has to end the child's
	// whole process group, and CommandContext kills the direct child only.
	cmd := exec.Command(name, args...) // #nosec G204 -- name is the adapter's configured binary; args are built by the adapter, no shell
	cmd.Dir = in.WorktreePath
	cmd.Env = procenv.ScrubNestedSession(procenv.Merge(in.Env))
	startInOwnGroup(cmd)

	// Pipes of our own rather than StdoutPipe: Wait runs beside the reads here,
	// and when a stop's bound is up the read ends are closed by us, so an
	// orphan still holding the write end cannot keep the action open.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return -1, "", fmt.Errorf("stdout pipe: %w", err)
	}
	defer func() { _ = stdoutR.Close() }()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutW.Close()
		return -1, "", fmt.Errorf("stderr pipe: %w", err)
	}
	defer func() { _ = stderrR.Close() }()
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	startErr := cmd.Start()
	// The child has its own copies of the write ends; with ours closed, the
	// reads end when the last process holding one lets go.
	_ = stdoutW.Close()
	_ = stderrW.Close()
	if startErr != nil {
		return -1, "", fmt.Errorf("start %s: %w", name, startErr)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	stderrCh := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(stderrR)
		stderrCh <- string(buf)
	}()

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdoutR)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4 MiB max line — CLIs can emit large JSON events
		for scanner.Scan() {
			onLine(scanner.Text())
		}
	}()

	var waitErr error
	exited, outDone := false, false
	outCh := (<-chan struct{})(stdoutDone)
	for !exited || !outDone {
		select {
		case waitErr = <-waitCh:
			exited = true
		case <-outCh:
			outDone = true
			outCh = nil
		case <-ctx.Done():
			deadline := time.Now().Add(stopBound)
			groupExited := stopGroup(cmd.Process, waitCh, exited, deadline)
			// Output already written is still read until the bound; then the
			// read ends close, whoever still holds the write ends.
			select {
			case <-stdoutDone:
			case <-time.After(time.Until(deadline)):
			}
			_ = stdoutR.Close()
			_ = stderrR.Close()
			<-stdoutDone
			return -1, <-stderrCh, &cutError{groupExited: groupExited, cause: ctx.Err()}
		}
	}
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

// stopGroup ends the process group p leads: it asks the group to end at once,
// kills it when the last tenth of the bound begins, and returns when the group
// is seen gone or the deadline passes. exited says whether the direct child
// was already reaped. Where there are no process groups, the direct child is
// killed and the group is never reported gone.
func stopGroup(p *os.Process, waitCh <-chan error, exited bool, deadline time.Time) (groupExited bool) {
	killAt := deadline.Add(-stopBound / 10)
	signalGroup(p, false)
	killed := false
	tick := time.NewTicker(stopPoll)
	defer tick.Stop()
	for {
		if !exited {
			select {
			case <-waitCh:
				exited = true
			default:
			}
		}
		if exited && groupGone(p) {
			return true
		}
		if exited && !processGroups {
			return false
		}
		now := time.Now()
		if !now.Before(deadline) {
			return false
		}
		if !killed && !now.Before(killAt) {
			signalGroup(p, true)
			killed = true
		}
		<-tick.C
	}
}

// binaryAvailable returns true if a binary is found on PATH.
func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
