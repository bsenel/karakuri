package cliagent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var projectDirUnsafe = regexp.MustCompile(`[^A-Za-z0-9]`)

// sessionLog is where the CLI keeps the log of session id for a run in
// worktree: <config dir>/projects/<worktree, non-alphanumerics as "-">/<id>.jsonl.
// "" when the session or the config directory is unknown.
func (c *ClaudeCode) sessionLog(worktree, id string) string {
	if id == "" || worktree == "" || id != filepath.Base(id) {
		return ""
	}
	dir := c.configDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "projects", projectDirUnsafe.ReplaceAllString(worktree, "-"), id+".jsonl")
}

// untilLoggedReset is how long from now until the "resetsAt" (Unix seconds) of
// the last usage_limit_reached line of the log at path; 0 when there is no log,
// it cannot be read, or that reset is not in the future.
func untilLoggedReset(path string, now time.Time) time.Duration {
	if path == "" {
		return 0
	}
	f, err := os.Open(path) // #nosec G304 -- built from the CLI's own config directory
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	var resetsAt int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var line struct {
			APIError       string `json:"apiError"`
			APIErrorParams struct {
				Info struct {
					ResetsAt int64 `json:"resetsAt"`
				} `json:"rate_limit_info"`
			} `json:"apiErrorParams"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.APIError != "usage_limit_reached" {
			continue
		}
		resetsAt = line.APIErrorParams.Info.ResetsAt
	}
	if resetsAt == 0 {
		return 0
	}
	if d := time.Unix(resetsAt, 0).Sub(now); d > 0 {
		return d
	}
	return 0
}
