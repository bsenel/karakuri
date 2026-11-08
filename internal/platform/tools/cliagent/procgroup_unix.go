//go:build unix

package cliagent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// processGroups says whether this platform lets a stop reach the child's own
// children and see them gone.
const processGroups = true

// startInOwnGroup makes the child the leader of a new process group, so a stop
// can signal the child and every process it started that stayed in its group.
func startInOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends SIGTERM (kill false) or SIGKILL (kill true) to the whole
// process group p leads.
func signalGroup(p *os.Process, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-p.Pid, sig)
}

// groupGone reports whether no process is left in the group p leads. A killed
// member stays in the group until it is reaped.
func groupGone(p *os.Process) bool {
	return errors.Is(syscall.Kill(-p.Pid, 0), syscall.ESRCH)
}
