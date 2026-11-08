//go:build unix

// Process groups are a Unix feature, so these tests run on Unix only; the
// roadmap's Phase 37 step 2 says the stop bound does not hold elsewhere.

package cliagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stopScript is a delegated coding agent that starts a child of its own. Both
// append a line to the output file ($1) in a loop, and the script writes both
// PIDs to the pid file ($2). The child inherits the script's standard output,
// so it holds the pipe runStreaming reads after the script is dead. With
// ignoreTerm both processes ignore the polite signal.
func stopScript(t *testing.T, ignoreTerm bool) string {
	t.Helper()
	trap := ""
	if ignoreTerm {
		trap = "trap '' TERM\n"
	}
	script := "#!/bin/sh\n" + trap +
		"out=\"$1\"\npidfile=\"$2\"\n" +
		"( " + strings.TrimSuffix(trap, "\n") + "\n  while :; do echo child >> \"$out\"; sleep 0.05; done ) &\n" +
		"child=$!\n" +
		"echo \"$$ $child\" > \"$pidfile.tmp\"\nmv \"$pidfile.tmp\" \"$pidfile\"\n" +
		"while :; do echo parent >> \"$out\"; sleep 0.05; done\n"
	bin := filepath.Join(t.TempDir(), "stop-agent")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the script must be executable
		t.Fatalf("chmod script: %v", err)
	}
	return bin
}

type streamResult struct {
	exitCode int
	err      error
}

// startStopRun runs the script through runStreaming, waits until it has
// written both PIDs and the output file is growing, and returns the PIDs, the
// output file and the channel runStreaming's result arrives on. Any process
// still alive when the test ends is killed, so a red test leaks nothing.
func startStopRun(t *testing.T, ctx context.Context, ignoreTerm bool) ([]int, string, <-chan streamResult) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.log")
	pidFile := filepath.Join(dir, "pids")
	bin := stopScript(t, ignoreTerm)

	done := make(chan streamResult, 1)
	go func() {
		code, _, err := runStreaming(ctx, DelegateInput{WorktreePath: dir}, bin, []string{out, pidFile}, func(string) {})
		done <- streamResult{exitCode: code, err: err}
	}()

	pids := waitForPIDs(t, pidFile)
	t.Cleanup(func() {
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	waitForGrowth(t, out)
	return pids, out, done
}

func waitForPIDs(t *testing.T, pidFile string) []int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile) // #nosec G304 -- the path is under t.TempDir()
		if err == nil {
			fields := strings.Fields(string(raw))
			if len(fields) == 2 {
				pids := make([]int, 0, 2)
				for _, f := range fields {
					pid, convErr := strconv.Atoi(f)
					if convErr != nil {
						t.Fatalf("pid file holds %q", raw)
					}
					pids = append(pids, pid)
				}
				return pids
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the script did not write its PIDs within 5s")
	return nil
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func waitForGrowth(t *testing.T, path string) {
	t.Helper()
	first := fileSize(t, path)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fileSize(t, path) > first && first > 0 {
			return
		}
		if first == 0 {
			first = fileSize(t, path)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the output file did not grow within 5s")
}

// waitForResult returns runStreaming's result, or false when it has not
// returned within limit.
func waitForResult(done <-chan streamResult, limit time.Duration) (streamResult, bool) {
	select {
	case r := <-done:
		return r, true
	case <-time.After(limit):
		return streamResult{}, false
	}
}

// processGone reports whether signal 0 finds no such process.
func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// assertProcessesGone polls until limit after cancelledAt for every PID to be
// gone and reports the ones still alive.
func assertProcessesGone(t *testing.T, pids []int, cancelledAt time.Time, limit time.Duration) bool {
	t.Helper()
	deadline := cancelledAt.Add(limit)
	for {
		var alive []int
		for _, pid := range pids {
			if !processGone(pid) {
				alive = append(alive, pid)
			}
		}
		if len(alive) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			t.Errorf("processes %v were still alive %s after the cancel", alive, limit)
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func assertCut(t *testing.T, r streamResult, wantGroupExited bool) {
	t.Helper()
	var cut *cutError
	if !errors.As(r.err, &cut) {
		t.Errorf("runStreaming returned err %v (exit %d), want a *cutError marking the run as cut", r.err, r.exitCode)
		return
	}
	if cut.groupExited != wantGroupExited {
		t.Errorf("cutError.groupExited = %v, want %v", cut.groupExited, wantGroupExited)
	}
}

// A stop ends the agent and the agent's own children even when both ignore
// the polite signal: runStreaming returns within the bound, both processes are
// gone, the output file stops growing, and the result is marked as cut with
// the group seen to exit.
func TestRunStreaming_StopKillsTheWholeGroupThatIgnoresTerm(t *testing.T) {
	const bound = 2 * time.Second
	const margin = 2 * time.Second
	saved := stopBound
	stopBound = bound
	t.Cleanup(func() { stopBound = saved })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pids, out, done := startStopRun(t, ctx, true)

	cancel()
	cancelledAt := time.Now()

	gone := assertProcessesGone(t, pids, cancelledAt, bound)
	r, returned := waitForResult(done, time.Until(cancelledAt.Add(bound+margin)))
	if !returned {
		t.Errorf("runStreaming had not returned %s after the cancel (bound %s)", time.Since(cancelledAt).Round(time.Millisecond), bound)
	}
	if gone {
		before := fileSize(t, out)
		time.Sleep(500 * time.Millisecond)
		if after := fileSize(t, out); after != before {
			t.Errorf("output file grew from %d to %d bytes after the processes were gone", before, after)
		}
	}
	if returned {
		assertCut(t, r, true)
	}
}

// orphanScript is a delegated coding agent that leaves a background process
// behind holding its standard error, writes its own PID and the orphan's to
// the pid file ($1), prints one line and exits 0 at once.
func orphanScript(t *testing.T) string {
	t.Helper()
	script := "#!/bin/sh\n" +
		"sleep 30 >/dev/null &\n" +
		"echo \"$$ $!\" > \"$1.tmp\"\nmv \"$1.tmp\" \"$1\"\n" +
		"echo done\n"
	bin := filepath.Join(t.TempDir(), "orphan-agent")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the script must be executable
		t.Fatalf("chmod script: %v", err)
	}
	return bin
}

// startOrphanRun runs orphanScript through runStreaming and waits until the
// script itself has exited. The orphan is killed when the test ends, so a red
// test leaks nothing.
func startOrphanRun(t *testing.T, ctx context.Context) <-chan streamResult {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pids")
	bin := orphanScript(t)

	done := make(chan streamResult, 1)
	go func() {
		code, _, err := runStreaming(ctx, DelegateInput{WorktreePath: dir}, bin, []string{pidFile}, func(string) {})
		done <- streamResult{exitCode: code, err: err}
	}()

	pids := waitForPIDs(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(pids[1], syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for !processGone(pids[0]) {
		if time.Now().After(deadline) {
			t.Fatal("the script had not exited 5s after writing its PIDs")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return done
}

// A child that exits normally ends the run within the bound even when a
// process it left behind still holds standard error, and the run is not cut.
func TestRunStreaming_OrphanHoldingStderrDoesNotBlockANormalExit(t *testing.T) {
	const bound = 2 * time.Second
	const margin = 2 * time.Second
	saved := stopBound
	stopBound = bound
	t.Cleanup(func() { stopBound = saved })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exitedAt := time.Now()
	done := startOrphanRun(t, ctx)

	r, returned := waitForResult(done, time.Until(exitedAt.Add(bound+margin)))
	if !returned {
		t.Fatalf("runStreaming had not returned %s after the child's normal exit (bound %s); an orphan holding stderr blocks it", time.Since(exitedAt).Round(time.Millisecond), bound)
	}
	if cut, _ := RunCut(r.err); cut {
		t.Errorf("runStreaming marked a normal exit as cut: %v", r.err)
	}
	if r.err != nil || r.exitCode != 0 {
		t.Errorf("runStreaming = (exit %d, err %v), want (0, nil)", r.exitCode, r.err)
	}
}

// A stop that arrives while runStreaming waits on an orphan's standard error,
// after the child has exited, still cuts the run within the bound.
func TestRunStreaming_StopWhileAnOrphanHoldsStderrCutsTheRun(t *testing.T) {
	const bound = 2 * time.Second
	const margin = 2 * time.Second
	saved := stopBound
	stopBound = 10 * time.Second // longer than the test waits, so only the stop can end the run
	t.Cleanup(func() { stopBound = saved })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startOrphanRun(t, ctx)

	cancel()
	cancelledAt := time.Now()

	r, returned := waitForResult(done, bound+margin)
	if !returned {
		t.Fatalf("runStreaming had not returned %s after the cancel; an orphan holding stderr blocks the stop", time.Since(cancelledAt).Round(time.Millisecond))
	}
	assertCut(t, r, true)
}

// A child that exits when asked ends the run well inside the bound.
func TestRunStreaming_StopOfAGroupThatHonoursTermReturnsQuickly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pids, _, done := startStopRun(t, ctx, false)

	cancel()
	cancelledAt := time.Now()

	r, returned := waitForResult(done, 3*time.Second)
	if !returned {
		t.Errorf("runStreaming had not returned 3s after the cancel; a group that honours the polite signal should end well inside the %s bound", stopGrace)
	}
	assertProcessesGone(t, pids, cancelledAt, 3*time.Second)
	if returned {
		assertCut(t, r, true)
	}
}
