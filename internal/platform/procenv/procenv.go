// Package procenv shapes the environment Karakuri hands to the CLIs it spawns.
//
// It exists because two unrelated packages spawn the same binary: the
// cli_agents tool adapter delegates coding tasks to `claude`, and the LLM
// provider falls back to `claude` when no API key is set. Both need the same
// answer about what the child may inherit, and a copy in each would be two
// answers waiting to drift apart.
package procenv

import (
	"os"
	"strings"
)

// Merge overlays per-call variables onto this process's environment.
//
// A spawned child needs PATH and HOME to run at all, and a config names only
// the variables it adds — usually one credential. An override replaces the
// inherited entry rather than appending a second one: `exec` passes the slice
// through, and which of two entries for the same name wins is platform
// behaviour nobody should have to know.
func Merge(extra map[string]string) []string {
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

// ScrubNestedSession strips the markers a Claude Code session exports into its
// child processes.
//
// Without this, a `claude` spawned from a server that was itself launched
// inside a session inherits CLAUDECODE and the session ids, and the child
// treats itself as a continuation of the parent's session rather than a fresh
// run.
//
// Credentials are deliberately untouched: ANTHROPIC_API_KEY is auth, not a
// session marker, and stripping it would send the child looking for
// credentials it was already handed.
func ScrubNestedSession(env []string) []string {
	out := env[:0:0]
	for _, e := range env {
		name := e
		if eq := strings.IndexByte(e, '='); eq > 0 {
			name = e[:eq]
		}
		if name == "CLAUDECODE" || strings.HasPrefix(name, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, e)
	}
	return out
}
