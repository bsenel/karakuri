package cliagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// The Claude Code CLI keeps each session as <config dir>/projects/<project>/
// <session id>.jsonl, where <project> is the run's working directory with every
// character that is not a letter or a digit written as "-". A refusal for quota
// is one line of it, carrying "apiError":"usage_limit_reached" and, under
// "apiErrorParams":{"rate_limit_info":{...}}, "resetsAt" as Unix seconds. Read
// on 2026-11-21 from the CLI's own logs on the development machine; the
// fixtures here are synthetic and hold only that shape.
const logSessionID = "sess-synthetic-1"

// refusalWithoutAReset is the CLI's refusal as the adapter sees it when the
// text names no time it can read: still a quota refusal, with no return time.
const refusalWithoutAReset = "You've hit your 5-hour limit · resets soon"

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// stubSessionCLI prints the stream-json init line that names the session, then
// the refusal, stderr included, and exits 1, as the CLI does on a quota refusal.
func stubSessionCLI(t *testing.T, refusal, stderr string) string {
	t.Helper()
	body := fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q}`+"\n"+
		`{"type":"assistant","message":{"content":[{"type":"text","text":%q}]}}`, logSessionID, refusal)
	script := "#!/bin/sh\ncat <<'OUT'\n" + body + "\nOUT\n"
	if stderr != "" {
		script += "cat >&2 <<'ERR'\n" + stderr + "\nERR\n"
	}
	script += "exit 1\n"
	bin := filepath.Join(t.TempDir(), "stub-cli")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the stub must be executable
		t.Fatalf("chmod stub: %v", err)
	}
	return bin
}

// sessionLogDir is where the CLI would keep the log of a run in worktree,
// under configDir.
func sessionLogDir(configDir, worktree string) string {
	return filepath.Join(configDir, "projects", nonAlnum.ReplaceAllString(worktree, "-"))
}

func quotaLine(resetsAt time.Time) string {
	return fmt.Sprintf(`{"type":"assistant","error":"rate_limit","isApiErrorMessage":true,`+
		`"apiError":"usage_limit_reached","apiErrorParams":{"rate_limit_info":{"status":"rejected",`+
		`"resetsAt":%d,"rateLimitType":"five_hour"}}}`, resetsAt.Unix())
}

// delegateWithLog runs the stub in a fresh worktree whose session log holds
// logContent (nothing is written when it is nil) and returns the error.
func delegateWithLog(t *testing.T, bin string, logContent *string) error {
	t.Helper()
	configDir, worktree := t.TempDir(), t.TempDir()
	if logContent != nil {
		dir := sessionLogDir(configDir, worktree)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir log dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, logSessionID+".jsonl"), []byte(*logContent), 0o600); err != nil {
			t.Fatalf("write log: %v", err)
		}
	}
	c := NewClaudeCode(bin)
	c.now = atRecordedRefusal
	c.configDir = configDir
	_, err := c.Delegate(context.Background(), DelegateInput{Prompt: "p", WorktreePath: worktree})
	if err == nil {
		t.Fatal("expected an error from a CLI that exits 1")
	}
	return err
}

func rateLimitOf(t *testing.T, err error) *coreerrors.ProviderRateLimitError {
	t.Helper()
	var limit *coreerrors.ProviderRateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error %q is not a *ProviderRateLimitError", err)
	}
	return limit
}

// Where the refusal's text gives no return time, the session log the CLI wrote
// for the run does, and the adapter reads it from there.
func TestClaudeCode_QuotaReturnTimeIsReadFromTheSessionLog(t *testing.T) {
	bin := stubSessionCLI(t, refusalWithoutAReset, "")
	log := quotaLine(atRecordedRefusal().Add(2*time.Hour)) + "\n"

	err := delegateWithLog(t, bin, &log)

	if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
		t.Fatalf("error %q is not ErrProviderRateLimited", err)
	}
	if got := rateLimitOf(t, err).RetryAfter; got != 2*time.Hour {
		t.Errorf("RetryAfter = %v, want 2h from the log's resetsAt", got)
	}
}

// A log is appended to: the refusal is read from the last quota line, not the
// first, and other lines around it are skipped.
func TestClaudeCode_QuotaReturnTimeComesFromTheLastQuotaLineOfTheLog(t *testing.T) {
	bin := stubSessionCLI(t, refusalWithoutAReset, "")
	log := quotaLine(atRecordedRefusal().Add(-time.Hour)) + "\n" +
		`{"type":"user","message":{"content":"synthetic"}}` + "\n" +
		quotaLine(atRecordedRefusal().Add(90*time.Minute)) + "\n" +
		`{"type":"system","subtype":"synthetic"}` + "\n"

	err := delegateWithLog(t, bin, &log)

	if got := rateLimitOf(t, err).RetryAfter; got != 90*time.Minute {
		t.Errorf("RetryAfter = %v, want 1h30m from the last quota line", got)
	}
}

// With no log, an unreadable log or a log that holds no reset, the refusal is
// the typed rate limit with RetryAfter 0, as before, so step 9's backoff
// applies; the CLI's own failure stays in the error.
func TestClaudeCode_QuotaWithoutAReadableLogResetKeepsNoRetryAfter(t *testing.T) {
	noReset := `{"type":"assistant","message":{"content":[{"type":"text","text":"synthetic"}]}}` + "\n"
	malformed := "{not json\n"
	pastReset := quotaLine(atRecordedRefusal().Add(-time.Hour)) + "\n"
	cases := []struct {
		name string
		log  *string
	}{
		{"no log", nil},
		{"empty log", new(string)},
		{"log without a quota line", &noReset},
		{"malformed log", &malformed},
		{"a reset already past", &pastReset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := stubSessionCLI(t, refusalWithoutAReset, "")

			err := delegateWithLog(t, bin, tc.log)

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("error %q is not ErrProviderRateLimited", err)
			}
			if got := rateLimitOf(t, err).RetryAfter; got != 0 {
				t.Errorf("RetryAfter = %v, want 0", got)
			}
			if want := "last output: " + refusalWithoutAReset; !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lost the CLI's last output %q", err, want)
			}
		})
	}
}

// A log that cannot be read at all (here, a directory where the file should
// be) is not a reason to fail the refusal or to invent a time.
func TestClaudeCode_QuotaWithAnUnreadableLogKeepsNoRetryAfter(t *testing.T) {
	bin := stubSessionCLI(t, refusalWithoutAReset, "")
	configDir, worktree := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(sessionLogDir(configDir, worktree), logSessionID+".jsonl"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	c := NewClaudeCode(bin)
	c.now = atRecordedRefusal
	c.configDir = configDir

	_, err := c.Delegate(context.Background(), DelegateInput{Prompt: "p", WorktreePath: worktree})

	if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
		t.Fatalf("error %v is not ErrProviderRateLimited", err)
	}
	if got := rateLimitOf(t, err).RetryAfter; got != 0 {
		t.Errorf("RetryAfter = %v, want 0", got)
	}
}

// The time the refusal itself gives is what the person read; the log is only
// for when it gives none.
func TestClaudeCode_QuotaResetInTheOutputWinsOverTheSessionLog(t *testing.T) {
	bin := stubSessionCLI(t, quotaRefusal, "")
	log := quotaLine(atRecordedRefusal().Add(2*time.Hour)) + "\n"

	err := delegateWithLog(t, bin, &log)

	if want := 3*time.Hour + 37*time.Minute + 43*time.Second; rateLimitOf(t, err).RetryAfter != want {
		t.Errorf("RetryAfter = %v, want %v from the output's own reset", rateLimitOf(t, err).RetryAfter, want)
	}
}
