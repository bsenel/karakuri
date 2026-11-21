package cliagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// quotaRefusal is what the Claude Code CLI printed when it refused for quota,
// as the audit log recorded it on 2026-11-14: on stdout, with an exit of 1 and
// nothing on stderr.
const quotaRefusal = "You've hit your 5-hour limit · resets 4:20pm (Europe/Paris)"

// atRecordedRefusal is a clock fixed at 2026-11-14T11:42:17Z, 12:42:17 in
// Paris (CET, UTC+1), so the next 16:20 there is 15:20:00Z.
func atRecordedRefusal() time.Time {
	return time.Date(2026, 11, 14, 11, 42, 17, 0, time.UTC)
}

func delegateWithClock(t *testing.T, bin string, now func() time.Time) error {
	t.Helper()
	c := NewClaudeCode(bin)
	c.now = now
	_, err := c.Delegate(context.Background(), DelegateInput{Prompt: "p", WorktreePath: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error from a CLI that exits 1")
	}
	return err
}

// The pass can only stop spending actions on a quota refusal it can tell
// apart from a failure, so the adapter names it as the domain's rate limit,
// with the time the quota returns.
func TestClaudeCode_QuotaRefusalIsATypedRateLimitWithItsResetTime(t *testing.T) {
	bin := newPrintingStubCLI(t, quotaRefusal, 1)

	err := delegateWithClock(t, bin, atRecordedRefusal)

	if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
		t.Fatalf("error %q is not ErrProviderRateLimited", err)
	}
	var limit *coreerrors.ProviderRateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error %q is not a *ProviderRateLimitError", err)
	}
	if limit.Provider != "claude_code" {
		t.Errorf("Provider = %q, want claude_code", limit.Provider)
	}
	if !strings.Contains(err.Error(), "last output: "+quotaRefusal) {
		t.Errorf("error %q no longer carries the CLI's last output", err)
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("error %q no longer names the exit status", err)
	}
	if want := 3*time.Hour + 37*time.Minute + 43*time.Second; limit.RetryAfter != want {
		t.Errorf("RetryAfter = %v, want %v (to 16:20 Europe/Paris)", limit.RetryAfter, want)
	}
}

// A refusal whose reset time is not in the form recorded on 2026-11-14 is
// still a quota refusal; it just gives no time, and RetryAfter says so with 0.
func TestClaudeCode_QuotaRefusalWithAnUnreadableResetHasNoRetryAfter(t *testing.T) {
	bin := newPrintingStubCLI(t, "You've hit your 5-hour limit · resets whenever the moon is full", 1)

	err := delegateWithClock(t, bin, atRecordedRefusal)

	if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
		t.Fatalf("error %q is not ErrProviderRateLimited", err)
	}
	var limit *coreerrors.ProviderRateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error %q is not a *ProviderRateLimitError", err)
	}
	if limit.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 for a reset it cannot read", limit.RetryAfter)
	}
}

// An adapter that cannot tell quota from another failure keeps reporting a
// failure, worded exactly as before; it never guesses from an empty string.
func TestClaudeCode_ExitWithoutAQuotaReasonIsAFailureAsToday(t *testing.T) {
	cases := []struct {
		name    string
		printed string
		want    string
	}{
		{
			name:    "other text",
			printed: "Error: the worktree is not a git repository",
			want:    "claude_code: exit 1 (stderr: ); last output: Error: the worktree is not a git repository",
		},
		{
			name:    "a limit that does not reset",
			printed: "You've hit your context window limit",
			want:    "claude_code: exit 1 (stderr: ); last output: You've hit your context window limit",
		},
		{
			name:    "nothing",
			printed: "",
			want:    "claude_code: exit 1 (stderr: )",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := newPrintingStubCLI(t, tc.printed, 1)

			err := delegateWithClock(t, bin, atRecordedRefusal)

			if errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Errorf("error %q is ErrProviderRateLimited; it gave no quota reason", err)
			}
			var limit *coreerrors.ProviderRateLimitError
			if errors.As(err, &limit) {
				t.Errorf("error %q is a *ProviderRateLimitError", err)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
		})
	}
}
