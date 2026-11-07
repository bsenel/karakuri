//go:build !unix

package cliagent

import (
	"os"
	"os/exec"
)

// processGroups is false here: there are no Unix process groups, so a stop
// kills the direct child only and cannot see the child's own children. The
// stop bound does not hold for them (docs/roadmap.md, Phase 37 step 2).
const processGroups = false

// startInOwnGroup does nothing: there is no process group to start.
func startInOwnGroup(*exec.Cmd) {}

// signalGroup kills the direct child, whether asked politely or not: there is
// no polite signal to send and no group to reach.
func signalGroup(p *os.Process, _ bool) {
	_ = p.Kill()
}

// groupGone never reports the group gone, because it cannot be seen.
func groupGone(*os.Process) bool { return false }
